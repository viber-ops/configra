package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/accessnats"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/logworker"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "configra",
		Short:         "Configra Server",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	for _, mode := range []bootstrap.Mode{bootstrap.Management, bootstrap.API} {
		mode := mode
		var configPath string
		command := &cobra.Command{
			Use:  string(mode),
			Args: cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				return run(command.Context(), mode, configPath)
			},
		}
		command.Flags().StringVar(&configPath, "config", "", "bootstrap YAML file")
		_ = command.MarkFlagRequired("config")
		root.AddCommand(command)
	}
	return root
}

func run(ctx context.Context, mode bootstrap.Mode, configPath string) error {
	config, err := bootstrap.Load(configPath, mode)
	if err != nil {
		return err
	}
	loggerConfig := zap.NewProductionConfig()
	if err := loggerConfig.Level.UnmarshalText([]byte(config.Logging.Level)); err != nil {
		return errors.New("invalid logging level")
	}
	logger, err := loggerConfig.Build()
	if err != nil {
		return errors.New("initialize Zap logger")
	}
	defer logger.Sync()
	undoStandardLog := zap.RedirectStdLog(logger.Named("stdlib"))
	defer undoStandardLog()

	masterKey, err := bootstrap.LoadMasterKey(config.KeyProvider.MasterKeyFile)
	if err != nil {
		return err
	}
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	clear(masterKey)
	if err != nil {
		return err
	}
	tlsConfig, err := bootstrap.LoadServerTLS(config.TLS, mode)
	if err != nil {
		return err
	}

	startupContext, cancelStartup := context.WithTimeout(ctx, 30*time.Second)
	defer cancelStartup()
	if mode == bootstrap.Management {
		clientCAs, err := bootstrap.LoadClientCAs(config.TLS.ClientCAFile)
		if err != nil {
			return err
		}
		store, err := mysqlstore.OpenManagement(startupContext, config.MySQLDSN(), provider)
		if err != nil {
			return err
		}
		defer store.Close()
		sessionStore := store.NewManagementSessionStore(5 * time.Minute)
		defer sessionStore.StopCleanup()
		sessions := humanauth.NewSessionManager(sessionStore)
		sessions.ErrorFunc = func(response http.ResponseWriter, request *http.Request, _ error) {
			logger.Error("Session operation failed",
				zap.String("method", request.Method),
				zap.String("path", request.URL.Path),
			)
			response.Header().Set("Cache-Control", "no-store")
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusInternalServerError)
			_, _ = response.Write([]byte(`{"error":{"code":"session_unavailable"}}`))
		}
		oidcConfig := config.OIDC
		authentication, err := humanauth.NewOIDC(startupContext, humanauth.OIDCConfig{
			Issuer:                oidcConfig.Issuer,
			ClientID:              oidcConfig.ClientID,
			ClientSecret:          config.OIDCClientSecret(),
			RedirectURL:           oidcConfig.RedirectURL,
			Scopes:                oidcConfig.Scopes,
			RoleSource:            humanauth.RoleSource(oidcConfig.RoleSource),
			RoleClaim:             oidcConfig.RoleClaim,
			ViewerValues:          oidcConfig.ViewerValues,
			AdminValues:           oidcConfig.AdminValues,
			AllowInsecureLoopback: oidcConfig.AllowInsecureLoopback,
		}, sessions)
		if err != nil {
			return err
		}
		logs, err := logstore.New(config.ClickHouseDSN())
		if err != nil {
			return err
		}
		defer logs.Close()
		consumer, err := accessnats.ConnectConsumer(accessnats.Config{
			URLs:            config.NATS.URLs,
			CredentialsFile: config.NATS.CredentialsFile,
			RootCAFile:      config.NATS.RootCAFile,
		}, logger)
		if err != nil {
			return err
		}
		notificationConfig := notification.Config{}
		if config.Notifications != nil {
			notificationConfig.AllowedInternalHosts = config.Notifications.AllowedInternalHosts
			notificationConfig.AllowedCIDRs = config.Notifications.AllowedCIDRs
			notificationConfig.RootCAs, err = bootstrap.LoadNotificationRootCAs(config.Notifications.RootCAFile)
			if err != nil {
				consumer.Close()
				return err
			}
		}
		notificationSender, err := notification.NewSender(notificationConfig)
		if err != nil {
			consumer.Close()
			return err
		}
		defer notificationSender.CloseIdleConnections()
		workerContext, cancelWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			if err := logworker.Run(workerContext, store, logs, consumer, notificationSender, logger); err != nil {
				logger.Error("Log Worker stopped unexpectedly")
			}
		}()
		defer func() {
			cancelWorker()
			<-workerDone
		}()
		handler := withHealth(authentication.Handler(management.NewHandler(store, consumer, clientCAs, logs)), store.Ping)
		return serve(ctx, config.Listen, tlsConfig, handler, logger, mode)
	}

	store, err := mysqlstore.OpenAPI(startupContext, config.MySQLDSN(), provider)
	if err != nil {
		return err
	}
	defer store.Close()
	publisher, err := accessnats.Connect(accessnats.Config{
		URLs:            config.NATS.URLs,
		CredentialsFile: config.NATS.CredentialsFile,
		RootCAFile:      config.NATS.RootCAFile,
	}, logger)
	if err != nil {
		return err
	}
	defer func() {
		flushContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if publisher.Close(flushContext) != nil {
			logger.Warn("NATS Access Event flush did not complete")
		}
	}()
	handler := withHealth(machine.NewHandler(store, publisher), store.Ping)
	return serve(ctx, config.Listen, tlsConfig, handler, logger, mode)
}

func withHealth(next http.Handler, ping func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health/ready", func(response http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		response.Header().Set("Cache-Control", "no-store")
		if ping(ctx) != nil {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", next)
	return mux
}

func serve(ctx context.Context, address string, tlsConfig *tls.Config, handler http.Handler, logger *zap.Logger, mode bootstrap.Mode) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          zap.NewStdLog(logger.Named("http")),
	}
	result := make(chan error, 1)
	go func() {
		result <- server.ServeTLS(listener, "", "")
	}()
	logger.Info("Server started", zap.String("mode", string(mode)), zap.String("address", listener.Addr().String()))

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTPS: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTPS: %w", err)
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTPS: %w", err)
		}
		logger.Info("Server stopped", zap.String("mode", string(mode)))
		return nil
	}
}
