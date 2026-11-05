// Package cli implements the configractl executable using Cobra and configra-go.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/spf13/cobra"
	configra "github.com/viber-ops/configra-go"
)

type commandError struct {
	code    int
	message string
}

func (err *commandError) Error() string   { return err.message }
func fail(code int, message string) error { return &commandError{code, message} }

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

type options struct {
	profile
	contextFile, contextName string
	timeout                  time.Duration
	json                     bool
	executing                bool
}

// Run emits only bounded, value-free errors. Sensitive outputs require an explicit
// output destination; usage/parse errors never echo arguments or input values.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	configHome, err := os.UserConfigDir()
	if err != nil {
		configHome = "."
	}
	settings := &options{}
	root := &cobra.Command{Use: "configractl", Short: "Scoped Configra configuration and credential operations", Version: version, SilenceErrors: true, SilenceUsage: true}
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return fail(2, "invalid_arguments") })
	var cancelCommand context.CancelFunc
	defer func() {
		if cancelCommand != nil {
			cancelCommand()
		}
	}()
	root.PersistentPreRunE = func(command *cobra.Command, _ []string) error {
		if command.ValidateRequiredFlags() != nil {
			return fail(2, "missing_required_arguments")
		}
		settings.executing = true
		if settings.timeout <= 0 {
			return fail(2, "positive_timeout_required")
		}
		ctx, cancel := context.WithTimeout(command.Context(), settings.timeout)
		cancelCommand = cancel
		command.SetContext(ctx)
		return nil
	}
	flags := root.PersistentFlags()
	flags.StringVar(&settings.Server, "server", "", "Configra API HTTPS origin")
	flags.StringVar(&settings.Environment, "environment", "", "Explicit Environment key")
	flags.StringVar(&settings.TokenFile, "token-file", "", "Token file; secret values are never flags")
	flags.StringVar(&settings.CertificateFile, "client-cert", "", "Client certificate PEM file")
	flags.StringVar(&settings.KeyFile, "client-key", "", "Client private key PEM file")
	flags.StringVar(&settings.ServerCAFile, "server-ca", "", "API server CA PEM file")
	flags.StringVar(&settings.contextFile, "context-file", filepath.Join(configHome, "configra", "contexts.json"), "Local context file (credential paths only)")
	flags.StringVar(&settings.contextName, "context", "", "Named local context")
	flags.DurationVar(&settings.timeout, "timeout", 30*time.Second, "Complete command deadline")
	flags.BoolVar(&settings.json, "json", false, "Compact machine-readable metadata output")
	root.AddCommand(contextCommand(settings), configCommand(settings), vaultCommand(settings), credentialCommand(settings), releaseCommand(settings))
	root.AddCommand(&cobra.Command{Use: "identity", Short: "Inspect this Token's scopes and effective certificate expiry", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			identity, err := client.ReadIdentity(ctx, selected.Environment)
			if err != nil {
				return err
			}
			return settings.emit(command, identity)
		})
	}})
	root.AddCommand(&cobra.Command{Use: "check CONFIG", Short: "Probe an actual authorized Config read without exposing its content", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			value, err := client.ReadResolvedConfig(ctx, selected.Environment, args[0], "")
			if err != nil {
				return err
			}
			return settings.emit(command, map[string]any{"status": "ok", "config_revision": value.ConfigRevision})
		})
	}})
	err = root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	code, message := 6, "operation_failed"
	var known *commandError
	var api *configra.APIError
	switch {
	case errors.As(err, &known):
		code, message = known.code, known.message
	case !settings.executing:
		code, message = 2, "invalid_arguments"
	case errors.As(err, &api):
		message = api.Code
		switch api.StatusCode {
		case 401, 403:
			code = 3
		case 409:
			code = 4
		case 400, 404, 422:
			code = 5
		default:
			code = 6
		}
	case errors.Is(err, context.Canceled):
		message = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		message = "timeout"
	}
	metadata := map[string]any{"error": message, "exit_code": code}
	if api != nil && api.RetryAfter > 0 {
		metadata["retry_after_seconds"] = int64(api.RetryAfter / time.Second)
	}
	_ = json.NewEncoder(stderr).Encode(metadata)
	return code
}

func resourceArgs(count int) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if len(args) != count {
			return fail(2, "invalid_arguments")
		}
		for _, value := range args {
			if !keyPattern.MatchString(value) {
				return fail(2, "invalid_resource_key")
			}
		}
		return nil
	}
}

func (settings *options) network(command *cobra.Command, action func(context.Context, *configra.Client, profile) error) error {
	selected, err := settings.connection(command)
	if err != nil {
		return err
	}
	if !keyPattern.MatchString(selected.Environment) || settings.timeout <= 0 {
		return fail(2, "environment_and_positive_timeout_required")
	}
	client, err := configra.NewClient(configra.ClientOptions{BaseURL: selected.Server, TokenFile: selected.TokenFile, ClientCertificateFile: selected.CertificateFile, ClientKeyFile: selected.KeyFile, ServerCAFile: selected.ServerCAFile, Timeout: settings.timeout})
	if err != nil {
		return fail(2, "invalid_connection_settings_or_credentials")
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(command.Context(), settings.timeout)
	defer cancel()
	return action(ctx, client, selected)
}

func (settings *options) emit(command *cobra.Command, result any) error {
	encoder := json.NewEncoder(command.OutOrStdout())
	if !settings.json {
		encoder.SetIndent("", "  ")
	}
	if encoder.Encode(result) != nil {
		return fail(7, "cannot_write_metadata")
	}
	return nil
}

func readInput(command *cobra.Command, path string, limit int64) ([]byte, error) {
	var reader io.Reader = command.InOrStdin()
	if path != "-" {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fail(7, "input_must_be_a_readable_regular_file")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fail(7, "cannot_read_input")
		}
		defer file.Close()
		reader = file
	}
	type readResult struct {
		value []byte
		err   error
	}
	done := make(chan readResult, 1)
	go func() {
		value, err := io.ReadAll(io.LimitReader(reader, limit+1))
		if command.Context().Err() != nil {
			clear(value)
			value = nil
		}
		done <- readResult{value, err}
	}()
	var value []byte
	var err error
	select {
	case result := <-done:
		value, err = result.value, result.err
	case <-command.Context().Done():
		if closer, ok := reader.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, command.Context().Err()
	}
	if command.Context().Err() != nil {
		clear(value)
		return nil, command.Context().Err()
	}
	if err != nil || int64(len(value)) > limit {
		clear(value)
		return nil, fail(7, "input_unreadable_or_too_large")
	}
	return value, nil
}

func privateOutput(command *cobra.Command, path string, value []byte, reveal bool) error {
	if path == "-" {
		if !reveal {
			return fail(2, "stdout_content_requires_reveal")
		}
		if _, err := command.OutOrStdout().Write(value); err != nil {
			return fail(7, "cannot_write_output")
		}
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail(7, "output_must_be_a_new_private_file")
	}
	_, writeErr := file.Write(value)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return fail(7, "cannot_write_output")
	}
	return nil
}

func outputFlags(command *cobra.Command, path *string, reveal *bool) {
	command.Flags().StringVar(path, "output", "", "New private output file, or - with --reveal")
	command.Flags().BoolVar(reveal, "reveal", false, "Explicitly allow sensitive content on stdout")
	_ = command.MarkFlagRequired("output")
}

func writeFlags(command *cobra.Command, revision *configra.RevisionWrite) {
	command.Flags().StringVar(&revision.OperationID, "operation-id", "", "Stable ID for this exact request; reuse after uncertain completion")
	command.Flags().Uint64Var(&revision.ExpectedRevision, "expected-revision", 0, "Current revision; explicit 0 creates a missing resource")
	_ = command.MarkFlagRequired("operation-id")
	_ = command.MarkFlagRequired("expected-revision")
}

func operationFlag(command *cobra.Command, value *string) {
	command.Flags().StringVar(value, "operation-id", "", "Stable operation identity")
	_ = command.MarkFlagRequired("operation-id")
}

func fileFlag(command *cobra.Command, value *string) {
	command.Flags().StringVar(value, "file", "", "Input file, or - for stdin")
	_ = command.MarkFlagRequired("file")
}

func safeJSON(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(2, "invalid_input_document")
	}
	return nil
}
