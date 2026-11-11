//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/vaultdoc"
	"github.com/viber-ops/configra/internal/vaultimport"
)

// A Vault Item import file is checked against the real Environment and Item tables, writes nothing, and its draft
// creates the Item through the ordinary PUT with the Fields in file order (#55, #54).
func TestVaultImportPreviewAgainstMySQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handler := management.NewHandler(newStore(t, ctx), nil, nil)
	call := func(method, path, operation string, body any) *httptest.ResponseRecorder {
		t.Helper()
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequest(method, "https://management.test"+path, bytes.NewReader(encoded)).
			WithContext(humanauth.WithPrincipal(ctx, humanauth.Principal{Subject: "admin", Role: humanauth.RoleAdmin, MFAVerified: true}))
		request.Header.Set("Content-Type", "application/json")
		if operation != "" {
			request.Header.Set("Idempotency-Key", operation)
		}
		request.RemoteAddr = "192.0.2.71:443"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	for _, key := range []string{"hk47", "hk180", "retired"} {
		if response := call("POST", "/v1/environments", "import-env-"+key, map[string]any{"key": key, "display_name": strings.ToUpper(key)}); response.Code != 200 {
			t.Fatal(response.Code, response.Body)
		}
	}
	if response := call("POST", "/v1/environments/retired/archive", "import-env-archive", map[string]any{}); response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}

	// Nineteen URL Fields, deliberately not in key order.
	var file strings.Builder
	file.WriteString("namespace: order_service\nitem: clients-conf\ndisplay_name: order_service_clients_conf\nenvironments:\n  - hk47\n  - hk180\nfields:\n")
	order := make([]string, 0, 19)
	for index := range 19 {
		key := fmt.Sprintf("service-%02d", (index*7)%19)
		order = append(order, key)
		fmt.Fprintf(&file, "  - key: %s\n    type: text\n    value: \"https://%s.example.com/api\"\n", key, key)
	}
	type preview struct {
		Problems []vaultimport.Problem `json:"problems"`
		Draft    *vaultimport.Draft    `json:"draft"`
	}
	check := func(content string) preview {
		t.Helper()
		response := call("POST", "/v1/vault-items/import-preview", "", map[string]string{"content": content})
		var result preview
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal(response.Code, response.Body)
		}
		return result
	}

	unavailable := check(strings.Replace(file.String(), "  - hk180\n", "  - hk180\n  - retired\n  - nowhere\n", 1))
	if want := []vaultimport.Problem{
		{Path: "environments[2]", Line: 7, Code: vaultimport.CodeEnvironmentArchived},
		{Path: "environments[3]", Line: 8, Code: vaultimport.CodeEnvironmentUnknown},
	}; !slices.Equal(unavailable.Problems, want) || unavailable.Draft != nil {
		t.Fatalf("unavailable Environments = %+v", unavailable)
	}

	clean := check(file.String())
	if len(clean.Problems) != 0 || clean.Draft == nil || len(clean.Draft.Fields) != 19 {
		t.Fatalf("clean preview = %+v", clean)
	}
	if response := call("GET", "/v1/vault-items/order_service/clients-conf", "", nil); response.Code != 404 {
		t.Fatalf("preview wrote the Item: %d %s", response.Code, response.Body)
	}

	// The UI turns the draft into the ordinary create request.
	values := make(map[string]vaultdoc.Value, len(clean.Draft.Values))
	for key, text := range clean.Draft.Values {
		values[key] = vaultdoc.Value{Text: &text}
	}
	snapshot := vaultdoc.Snapshot{Fields: clean.Draft.Fields, Variants: []vaultdoc.Variant{{ID: "0123456789abcdef0123456789abcdef", Environments: clean.Draft.Environments, Values: values}}}
	if response := call("PUT", "/v1/vault-items/order_service/clients-conf", "import-create", map[string]any{
		"display_name": clean.Draft.DisplayName, "expected_revision": 0, "snapshot": snapshot,
	}); response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	response := call("GET", "/v1/vault-items/order_service/clients-conf/values", "", nil)
	var item struct {
		DisplayName string            `json:"display_name"`
		Snapshot    vaultdoc.Snapshot `json:"snapshot"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &item) != nil {
		t.Fatal(response.Code, response.Body)
	}
	keys := make([]string, 0, len(item.Snapshot.Fields))
	for _, field := range item.Snapshot.Fields {
		keys = append(keys, field.Key)
	}
	if item.DisplayName != "order_service_clients_conf" || !slices.Equal(keys, order) {
		t.Fatalf("created Item = %q %v, want order %v", item.DisplayName, keys, order)
	}
	variant := item.Snapshot.Variants[0]
	if !slices.Equal(variant.Environments, []string{"hk180", "hk47"}) && !slices.Equal(variant.Environments, []string{"hk47", "hk180"}) {
		t.Fatalf("Variant Environments = %v", variant.Environments)
	}
	if got := *variant.Values["service-07"].Text; got != "https://service-07.example.com/api" {
		t.Fatalf("service-07 = %q", got)
	}

	// The same file now conflicts, and still does after the Item is archived.
	if again := check(file.String()); !slices.Equal(again.Problems, []vaultimport.Problem{{Path: "item", Line: 2, Code: vaultimport.CodeItemExists}}) {
		t.Fatalf("existing Item preview = %+v", again)
	}
	if response := call("POST", "/v1/vault-items/order_service/clients-conf/archive", "import-archive", map[string]any{}); response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	if again := check(file.String()); !slices.Equal(again.Problems, []vaultimport.Problem{{Path: "item", Line: 2, Code: vaultimport.CodeItemArchived}}) {
		t.Fatalf("archived Item preview = %+v", again)
	}
}
