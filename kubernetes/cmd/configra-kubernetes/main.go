package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"

	"github.com/viber-ops/configra/kubernetes/internal/binding"
	"github.com/viber-ops/configra/kubernetes/internal/provider"
	"github.com/viber-ops/configra/kubernetes/internal/source"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: configra-kubernetes provider|sync [flags]")
	}
	mode := os.Args[1]
	if mode != "provider" && mode != "sync" {
		return errors.New("mode must be provider or sync")
	}
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	origin := flags.String("configra-url", "", "trusted Configra API HTTPS origin")
	serverCA := flags.String("server-ca-file", "", "optional CA bundle for the Configra SERVER certificate")
	socket := flags.String("socket", "/provider/configra.sock", "CSI provider Unix socket")
	namespace := flags.String("namespace", "", "application namespace watched by the sync controller")
	health := flags.String("health-address", ":8081", "HTTP health probe address")
	leaderElection := flags.Bool("leader-elect", true, "elect one active sync controller per namespace")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	var rootCA []byte
	if *serverCA != "" {
		var err error
		rootCA, err = os.ReadFile(*serverCA)
		if err != nil {
			return errors.New("read Configra server CA bundle")
		}
	}
	reader, err := source.NewReader(*origin, rootCA)
	if err != nil {
		return err
	}
	ctx := ctrl.SetupSignalHandler()
	if mode == "provider" {
		return runProvider(ctx, reader, *socket, *health)
	}
	if *namespace == "" || len(validation.IsDNS1123Label(*namespace)) != 0 {
		return errors.New("sync requires one valid --namespace")
	}
	return runController(ctx, reader, *namespace, *health, *leaderElection)
}

func runProvider(ctx context.Context, reader source.Fetcher, socket, healthAddress string) error {
	if !filepath.IsAbs(socket) || filepath.Base(socket) != "configra.sock" {
		return errors.New("provider socket must be an absolute path ending in configra.sock")
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace a non-socket path")
		}
		if err := os.Remove(socket); err != nil {
			return errors.New("remove previous provider socket")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("inspect provider socket")
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return errors.New("listen on provider socket")
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0600); err != nil {
		return errors.New("restrict provider socket permissions")
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(4<<20), grpc.MaxConcurrentStreams(4))
	pb.RegisterCSIDriverProviderServer(server, &provider.Server{Source: reader})
	probe := &http.Server{Addr: healthAddress, ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" && request.URL.Path != "/readyz" {
			http.NotFound(response, request)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})}
	result := make(chan error, 2)
	go func() { result <- server.Serve(listener) }()
	go func() { result <- probe.ListenAndServe() }()
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-result:
	}
	server.Stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = probe.Shutdown(shutdown)
	if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) && !errors.Is(runErr, grpc.ErrServerStopped) {
		return errors.New("provider listener stopped")
	}
	return nil
}

func runController(ctx context.Context, reader source.Fetcher, namespace, healthAddress string, leaderElection bool) error {
	ctrl.SetLogger(zap.New())
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	scheme.AddKnownTypeWithName(binding.GVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(binding.GVK.GroupVersion().WithKind("ConfigraBindingList"), &unstructured.UnstructuredList{})
	metav1.AddToGroupVersion(scheme, binding.GVK.GroupVersion())
	config, err := ctrl.GetConfig()
	if err != nil {
		return errors.New("load Kubernetes client configuration")
	}
	manager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme, Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{namespace: {}}},
		Metrics: metrics.Options{BindAddress: "0"}, HealthProbeBindAddress: healthAddress,
		LeaderElection: leaderElection, LeaderElectionID: "configra-binding-controller", LeaderElectionNamespace: namespace,
	})
	if err != nil {
		return errors.New("initialize binding controller")
	}
	const credentialIndex = "spec.credentialsSecretRef.name"
	if err := manager.GetFieldIndexer().IndexField(ctx, binding.NewObject(), credentialIndex, func(object client.Object) []string {
		value, ok := object.(*unstructured.Unstructured)
		if !ok {
			return nil
		}
		name, _, _ := unstructured.NestedString(value.Object, "spec", "credentialsSecretRef", "name")
		if name == "" {
			return nil
		}
		return []string{name}
	}); err != nil {
		return errors.New("index binding credentials")
	}
	credentialsChanged := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, credential client.Object) []ctrl.Request {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(binding.GVK.GroupVersion().WithKind("ConfigraBindingList"))
		if manager.GetClient().List(ctx, list, client.InNamespace(namespace), client.MatchingFields{credentialIndex: credential.GetName()}) != nil {
			return nil
		}
		requests := make([]ctrl.Request, 0, len(list.Items))
		for _, item := range list.Items {
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: item.GetName()}})
		}
		return requests
	})
	if err := ctrl.NewControllerManagedBy(manager).
		For(binding.NewObject(), builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Secret{}).Owns(&corev1.ConfigMap{}).
		Watches(&corev1.Secret{}, credentialsChanged).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(&binding.Reconciler{Client: manager.GetClient(), APIReader: manager.GetAPIReader(), Source: reader, Namespace: namespace}); err != nil {
		return errors.New("register binding controller")
	}
	if err := manager.AddHealthzCheck("live", healthz.Ping); err != nil {
		return err
	}
	if err := manager.AddReadyzCheck("ready", healthz.Ping); err != nil {
		return err
	}
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("binding controller stopped: %w", err)
	}
	return nil
}
