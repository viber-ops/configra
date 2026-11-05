package source_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/kubernetes/internal/source"
)

func TestReaderVerifiesHTTPSAndDoesNotReturnPartialResults(t *testing.T) {
	token := "cfg_testuser_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	failed := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing workload authentication")
		}
		if strings.Contains(request.URL.Path, "/vault-items/") {
			if failed {
				response.WriteHeader(http.StatusInternalServerError)
				io.WriteString(response, "private-upstream-sentinel")
				return
			}
			response.Header().Set("ETag", `"file-1"`)
			response.Header().Set("Content-Type", "application/octet-stream")
			response.Header().Set("Content-Disposition", `attachment; filename="data.bin"`)
			response.Write([]byte{0, 1, 2})
			return
		}
		response.Header().Set("ETag", `"config-2"`)
		response.Header().Set("Content-Type", "application/json")
		json.NewEncoder(response).Encode(map[string]any{"format": "yaml", "content": "value: resolved\n", "config_revision": 2, "vault_revisions": map[string]int{"platform.credentials": 2}})
	}))
	defer server.Close()
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	reader, err := source.NewReader(server.URL, root)
	if err != nil {
		t.Fatal(err)
	}
	objects := []source.Object{{Type: "config", Environment: "production", Config: "app", Path: "app.yaml"}, {Type: "file", Environment: "production", Namespace: "platform", Item: "app", Field: "file", Path: "data.bin"}}
	credentials := map[string][]byte{"token": []byte(token)}
	values, err := reader.Read(context.Background(), objects, credentials)
	if err != nil || len(values) != 2 || !values[0].Sensitive || !values[1].Sensitive || string(values[0].Bytes) != "value: resolved\n" || len(values[1].Bytes) != 3 {
		t.Fatalf("read failed: %v", err)
	}
	failed = true
	values, err = reader.Read(context.Background(), objects, credentials)
	if err == nil || values != nil || strings.Contains(err.Error(), "private-upstream-sentinel") {
		t.Fatal("failure returned partial content or disclosed an error body")
	}
	untrusted, err := source.NewReader(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := untrusted.Read(context.Background(), objects[:1], credentials); err == nil {
		t.Fatal("accepted an untrusted HTTPS server")
	}
}

func TestReaderRejectsOriginsWithCredentialsAndUnsupportedSchemes(t *testing.T) {
	for _, value := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/path", "https://example.com?token=secret"} {
		if _, err := source.NewReader(value, nil); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsafe origin accepted or echoed")
		}
	}
}

func TestObjectPathsCannotConflictWithTheirDirectories(t *testing.T) {
	for _, paths := range [][]string{{"database", "database/client.pem"}, {"database/client.pem", "database"}} {
		objects := []source.Object{
			{Environment: "production", Config: "app", Path: paths[0]},
			{Environment: "production", Config: "app", Path: paths[1]},
		}
		if err := source.ValidateObjects(objects); err == nil {
			t.Fatal("accepted a path as both a file and directory")
		}
	}
	if err := source.ValidateObjects([]source.Object{
		{Environment: "production", Config: "app", Path: "database"},
		{Environment: "production", Config: "app", Path: "database-backup/client.pem"},
	}); err != nil {
		t.Fatalf("rejected non-conflicting paths: %v", err)
	}
}

func TestReleaseSourceFetchesOneCompleteSetAndRejectsMixedSources(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/environments/prod/configs/server/release" {
			t.Error("release used independent reads")
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", `"release-test"`)
		io.WriteString(w, `{"manifest":{"environment":"prod","config_key":"server","release_key":"stable","config_revision":1,"config_path":"server.yaml","files":[{"namespace":"ops","item":"app","field":"key","path":"key.pem","revision":1}],"vault_revisions":{"ops.app":1}},"generation":1,"digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","config":{"format":"yaml","content":"version: 1\n","config_revision":1,"vault_revisions":{"ops.app":1}},"files":[{"path":"key.pem","bytes":"cHJpdmF0ZQ=="}]}`)
	}))
	defer server.Close()
	reader, err := source.NewReader(server.URL, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	if err != nil {
		t.Fatal(err)
	}
	objects := []source.Object{{Type: "release", Environment: "prod", Config: "server", Path: "."}}
	values, err := reader.Read(context.Background(), objects, map[string][]byte{"token": []byte("cfg_testuser_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)))})
	if err != nil || len(values) != 2 || requests != 1 || values[0].Path != "server.yaml" || values[1].Path != "key.pem" || !values[1].Sensitive || values[0].Version != values[1].Version {
		t.Fatal("incomplete release material", err)
	}
	if err := source.ValidateObjects(append(objects, source.Object{Type: "config", Environment: "prod", Config: "another", Path: "another.yaml"})); err == nil {
		t.Fatal("release may be mixed with independently versioned objects")
	}
}
