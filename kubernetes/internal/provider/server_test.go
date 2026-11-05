package provider_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/viber-ops/configra/kubernetes/internal/provider"
	"github.com/viber-ops/configra/kubernetes/internal/source"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

type fakeSource struct {
	calls       int
	err         error
	credentials map[string][]byte
	materials   []source.Material
}

func (upstream *fakeSource) Read(_ context.Context, objects []source.Object, credentials map[string][]byte) ([]source.Material, error) {
	upstream.calls++
	upstream.credentials = map[string][]byte{}
	for key, value := range credentials {
		upstream.credentials[key] = append([]byte(nil), value...)
	}
	if upstream.err != nil {
		return nil, upstream.err
	}
	if upstream.materials != nil {
		return upstream.materials, nil
	}
	return []source.Material{{Path: objects[0].Path, Version: `"revision-2"`, Bytes: []byte("PORT: 8080\n")}}, nil
}

func TestProviderReturnsCompleteReleaseAndRejectsInconsistentVersion(t *testing.T) {
	upstream := &fakeSource{materials: []source.Material{{Path: "app.yaml", Version: "release-1", Bytes: []byte("version: 1\n")}, {Path: "secrets/key.pem", Version: "release-1", Bytes: []byte("private-key"), Sensitive: true}}}
	server := &provider.Server{Source: upstream}
	request := &pb.MountRequest{Attributes: `{"objects":"- type: release\n  environment: prod\n  config: app\n  path: .\n"}`, Secrets: `{}`}
	result, err := server.Mount(context.Background(), request)
	if err != nil || len(result.Files) != 2 || len(result.ObjectVersion) != 2 || result.Files[1].Path != "secrets/key.pem" {
		t.Fatal("incomplete release mount", err)
	}
	upstream.materials[1].Version = "release-2"
	if result, err := server.Mount(context.Background(), request); err == nil || result != nil {
		t.Fatal("provider returned mixed release")
	}
}

func TestProviderWireContractAndRestrictedPaths(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	upstream := &fakeSource{}
	pb.RegisterCSIDriverProviderServer(server, &provider.Server{Source: upstream})
	go server.Serve(listener)
	defer server.Stop()
	connection, err := grpc.NewClient("passthrough:///provider", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := pb.NewCSIDriverProviderClient(connection)
	version, err := client.Version(context.Background(), &pb.VersionRequest{})
	if err != nil || version.RuntimeName != "configra" || version.Version != "v1alpha1" {
		t.Fatal("invalid provider Version contract")
	}
	attributes := `{"objects":"- type: config\n  environment: production\n  config: app\n  path: app.yaml\n"}`
	mounted, err := client.Mount(context.Background(), &pb.MountRequest{Attributes: attributes, Secrets: `{"token":"credential-sentinel"}`, TargetPath: "/must-not-be-written"})
	if err != nil || len(mounted.Files) != 1 || mounted.Files[0].Mode != 0440 || mounted.Files[0].Path != "app.yaml" || string(mounted.Files[0].Contents) != "PORT: 8080\n" || mounted.ObjectVersion[0].Version != `"revision-2"` {
		t.Fatalf("mount contract failed: %v", err)
	}
	if string(upstream.credentials["token"]) != "credential-sentinel" {
		t.Fatal("workload credentials were not forwarded to the configured source")
	}
	for _, path := range []string{"../escape", "/absolute", "..data", "a/../b", "a//b", "a/..secret", `a\\b`} {
		request := strings.Replace(attributes, "app.yaml", path, 1)
		if _, err := client.Mount(context.Background(), &pb.MountRequest{Attributes: request, Secrets: `{}`}); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
	if upstream.calls != 1 {
		t.Fatal("invalid mapping reached the source")
	}
	for _, forbidden := range []string{`"fileMode":"0777",`, `"baseURL":"https://attacker.example",`} {
		invalid := "{" + forbidden + strings.TrimPrefix(attributes, "{")
		if _, err := client.Mount(context.Background(), &pb.MountRequest{Attributes: invalid, Secrets: `{}`}); err == nil {
			t.Fatal("accepted unsafe permissions or upstream override")
		}
	}
	readable := `{"fileMode":"0444",` + strings.TrimPrefix(attributes, "{")
	if mounted, err := client.Mount(context.Background(), &pb.MountRequest{Attributes: readable, Secrets: `{}`}); err != nil || mounted.Files[0].Mode != 0444 {
		t.Fatal("explicit read-only Pod permissions were not applied")
	}
	upstream.err = errors.New("private-value-must-not-appear")
	if response, err := client.Mount(context.Background(), &pb.MountRequest{Attributes: attributes, Secrets: `{}`}); err == nil || response != nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("failed fetch disclosed values or returned partial files")
	}
}
