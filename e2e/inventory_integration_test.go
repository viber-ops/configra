//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/management"
)

func newInventoryHTTP(t *testing.T, ctx context.Context) func(string, string, any, string, int) []byte {
	t.Helper()
	handler := management.NewHandler(newStore(t, ctx), nil, nil)
	// The external human identity adapter is a fixture; all reads, writes,
	// encryption, authorization and query parsing use the real HTTPS handler.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := humanauth.RoleAdmin
		if r.Header.Get("X-Fixture-Role") == "viewer" {
			role = humanauth.RoleViewer
		}
		handler.ServeHTTP(w, r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "inventory-http", Role: role})))
	}))
	t.Cleanup(server.Close)
	operation := 0
	call := func(method, path string, body any, role string, want int) []byte {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", server.URL)
		r.Header.Set("X-Fixture-Role", role)
		operation++
		r.Header.Set("Idempotency-Key", fmt.Sprintf("inventory-http-%d", operation))
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s %s = %d %s, %v", method, path, response.StatusCode, data, err)
		}
		return data
	}
	return call
}

func TestManagementSecurityInventoryHTTPAndOffPageVaultImpact(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	call := newInventoryHTTP(t, ctx)
	call("POST", "/v1/environments", map[string]any{"key": "production", "display_name": "Production"}, "admin", 200)
	call("PUT", "/v1/vault-items/platform/db", map[string]any{"display_name": "Database", "snapshot": map[string]any{
		"fields":   []map[string]any{{"key": "password", "name": "Password", "type": "secret"}, {"key": "host", "name": "Host", "type": "text"}},
		"variants": []map[string]any{{"environments": []string{"production"}, "values": map[string]any{"password": map[string]any{"text": "http-security-secret"}, "host": map[string]any{"text": "localhost"}}}},
	}}, "admin", 200)
	events := make([]string, 105)
	for index := range events {
		events[index] = fmt.Sprintf("event.%03d", index)
	}
	var lastID string
	for index := range events {
		name := fmt.Sprintf("entry_%03d", index)
		data := call("POST", "/v1/certificate-authorities", map[string]any{"display_name": name, "valid_days": 365}, "admin", 200)
		var created struct {
			Authority struct {
				ID string `json:"id"`
			} `json:"authority"`
		}
		if err := json.Unmarshal(data, &created); err != nil {
			t.Fatal(err)
		}
		lastID = created.Authority.ID
		call("POST", "/v1/client-certificates/issue", map[string]any{"authority_id": lastID, "display_name": name, "valid_days": 30}, "admin", 200)
		if index < len(events)-32 {
			call("POST", "/v1/certificate-authorities/"+lastID+"/revoke", nil, "admin", 200)
		}
		call("PUT", "/v1/notification-destinations/"+name, map[string]any{"display_name": name, "provider": "generic_webhook", "url": "https://hooks.example.com/private-url-token", "secret": "http-security-secret", "enabled": true, "event_types": events}, "admin", 200)
		field := "password"
		if index == len(events)-1 {
			field = "host"
		}
		call("PUT", "/v1/environments/production/configs/"+name, map[string]any{"name": name, "format": "yaml", "content": "value: '{vault.platform.db." + field + "}'\n"}, "admin", 200)
	}
	type page struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	decode := func(data []byte) page {
		t.Helper()
		var result page
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, path := range []string{"/v1/certificate-authorities?include_revoked=true", "/v1/client-certificates?include_revoked=true", "/v1/notification-destinations?include_archived=true", "/v1/notification-destinations/entry_000/event-types?", "/v1/vault-items/platform/db/usages?"} {
		seen := map[string]bool{}
		for _, offset := range []int{0, 50, 100, 150} {
			data := call("GET", fmt.Sprintf("%s&offset=%d", path, offset), nil, "admin", 200)
			result := decode(data)
			if result.Total != 105 || len(result.Items) != max(0, min(50, 105-offset)) || len(data) > 150000 {
				t.Fatalf("unbounded or truncated HTTP page %s", path)
			}
			for _, item := range result.Items {
				if seen[string(item)] {
					t.Fatal("duplicate across stable pages")
				}
				seen[string(item)] = true
			}
			for _, forbidden := range []string{"http-security-secret", "private-url-token", "PRIVATE KEY", "export_bundle", "ciphertext", "encrypted_dek"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("list exposed %q", forbidden)
				}
			}
		}
		if len(seen) != 105 {
			t.Fatal("incomplete page traversal")
		}
		if !strings.Contains(path, "/usages") {
			call("GET", path, nil, "viewer", 403)
		}
	}
	usable := decode(call("GET", "/v1/certificate-authorities?usable=true", nil, "admin", 200))
	if usable.Total != 32 || len(usable.Items) != 32 {
		t.Fatal("issuable Authorities lost behind historical pages")
	}
	certs := decode(call("GET", "/v1/client-certificates?q=entry_104", nil, "admin", 200))
	if certs.Total != 1 || !strings.Contains(string(certs.Items[0]), `"authority_name":"entry_104"`) {
		t.Fatal("issuer name needs a full Authority list")
	}
	path := "/v1/vault-items/platform/db/impact-preview"
	body := map[string]any{"fields": []string{"password"}, "environments": []string{"production"}}
	impact := decode(call("POST", path+"?limit=1", body, "admin", 200))
	if impact.Total != 1 || len(impact.Items) != 1 || !strings.Contains(string(impact.Items[0]), `"config_key":"entry_104"`) {
		t.Fatal("off-page impact missed")
	}
	call("POST", path, body, "viewer", 403)
	call("POST", path+"?q=unrelated", body, "admin", 400)
	call("POST", path, map[string]any{"fields": []string{"password"}}, "admin", 400)
	call("POST", path, map[string]any{"fields": []string{"password", "password"}, "environments": []string{}}, "admin", 400)
	body["environments"] = []string{}
	impact = decode(call("POST", path+"?limit=1&offset=100", body, "admin", 200))
	if impact.Total != 105 || len(impact.Items) != 1 {
		t.Fatal("environment impact truncated")
	}
	call("POST", "/v1/vault-items/elsewhere/db/impact-preview", body, "admin", 404)
	call("GET", "/v1/notification-destinations/missing/event-types", nil, "admin", 404)
	for _, query := range []string{"usable=", "usable=yes", "usable=true&status=revoked", "limit=101", "limit=1&limit=2"} {
		call("GET", "/v1/certificate-authorities?"+query, nil, "admin", 400)
	}
	t.Log("105-row HTTPS lists, complete traversal, issuer metadata, Admin/Viewer boundaries and off-page impacts passed with MySQL 8.0.22; human identity is an external fixture")
}

func TestManagementInventoryHTTPPagesAndIncrementalGrants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	call := newInventoryHTTP(t, ctx)
	const count = 105
	keys := make([]string, count)
	for index := range keys {
		keys[index] = fmt.Sprintf("resource_%03d", index)
		call("POST", "/v1/environments", map[string]any{"key": keys[index], "display_name": keys[index]}, "admin", 200)
	}
	var tokenID, tokenSecret string
	for index, key := range keys {
		call("PUT", "/v1/environments/"+keys[0]+"/configs/"+key, map[string]any{"name": key, "format": "yaml", "expected_revision": 0, "content": "secret: http-source-sentinel\n"}, "admin", 200)
		call("PUT", "/v1/vault-items/platform/"+key, map[string]any{"display_name": key, "expected_revision": 0, "snapshot": map[string]any{
			"fields":   []map[string]any{{"key": "password", "name": "Password", "type": "secret"}},
			"variants": []map[string]any{{"environments": keys, "values": map[string]any{"password": map[string]any{"text": "http-vault-sentinel"}}}},
		}}, "admin", 200)
		data := call("POST", "/v1/api-tokens", map[string]any{"display_name": key, "environment_keys": keys}, "admin", 200)
		if index == 0 {
			var result struct {
				PublicID string `json:"public_id"`
				Token    string `json:"token"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			tokenID, tokenSecret = result.PublicID, result.Token
		}
	}
	type record struct {
		Key              string            `json:"key"`
		PublicID         string            `json:"public_id"`
		Environments     []json.RawMessage `json:"environments"`
		EnvironmentKeys  []string          `json:"environment_keys"`
		EnvironmentCount int               `json:"environment_count"`
		Granted          bool              `json:"granted"`
	}
	type page struct {
		Items []record `json:"items"`
		Total int      `json:"total"`
	}
	read := func(path, role string) page {
		t.Helper()
		data := call("GET", path, nil, role, 200)
		for _, value := range []string{"http-source-sentinel", "http-vault-sentinel", tokenSecret, `"secret_digest":`, `"snapshot":`} {
			if value != "" && strings.Contains(string(data), value) {
				t.Fatal("inventory HTTP response leaked value material")
			}
		}
		var result page
		if err := json.Unmarshal(data, &result); err != nil || result.Items == nil {
			t.Fatalf("page = %s, %v", data, err)
		}
		return result
	}
	for _, resource := range []string{"environments", "configs", "vault-items", "api-tokens"} {
		role := "viewer"
		if resource == "api-tokens" {
			role = "admin"
		}
		seen := map[string]bool{}
		for offset := 0; offset <= 150; offset += 50 {
			result := read(fmt.Sprintf("/v1/%s?offset=%d", resource, offset), role)
			if result.Total != count || len(result.Items) != min(50, max(0, count-offset)) {
				t.Fatalf("%s page count = %d total=%d", resource, len(result.Items), result.Total)
			}
			for _, item := range result.Items {
				identity := item.Key
				if resource == "api-tokens" {
					identity = item.PublicID
				}
				if identity == "" || seen[identity] {
					t.Fatalf("%s duplicated/missing identity %q", resource, identity)
				}
				seen[identity] = true
				if len(item.Environments) > 3 || len(item.EnvironmentKeys) > 3 {
					t.Fatal("unbounded HTTP association summary")
				}
				if (resource == "api-tokens" || resource == "vault-items") && item.EnvironmentCount != count {
					t.Fatalf("lost association total: %#v", item)
				}
			}
		}
		if len(seen) != count {
			t.Fatalf("%s missed identities", resource)
		}
	}
	if result := read("/v1/configs?key="+keys[count-1], "viewer"); result.Total != 1 || result.Items[0].Key != keys[count-1] {
		t.Fatal("off-page identity lookup failed")
	}
	if result := read("/v1/vault-items?environment="+keys[count-1]+"&namespace=platform&limit=1", "viewer"); result.Total != count || len(result.Items) != 1 {
		t.Fatal("off-summary exact environment filter failed")
	}
	if result := read("/v1/vault-items?namespace=other&key="+keys[0], "viewer"); result.Total != 0 {
		t.Fatal("namespace filter leaked another namespace")
	}
	call("GET", "/v1/environments?token="+tokenID, nil, "viewer", 403)
	grants := read("/v1/environments?token="+tokenID+"&offset=100", "admin")
	if grants.Total != count || len(grants.Items) != 5 || !grants.Items[4].Granted {
		t.Fatal("off-page grant read failed")
	}
	patchPath := "/v1/api-tokens/" + tokenID + "/environments"
	change := map[string]any{"remove": []string{keys[0], keys[count-1]}}
	call("PATCH", patchPath, change, "viewer", 403)
	call("POST", "/v1/environments/"+keys[count-1]+"/archive", nil, "admin", 200)
	call("PATCH", patchPath, change, "admin", 200)
	call("PATCH", patchPath, change, "admin", 200)
	call("PATCH", patchPath, map[string]any{"add": []string{keys[count-1]}, "remove": []string{keys[1]}}, "admin", 422)
	if result := read("/v1/api-tokens?key="+tokenID, "admin"); result.Total != 1 || result.Items[0].EnvironmentCount != count-2 {
		t.Fatal("patch replaced untouched grants")
	}
	for _, key := range []string{keys[0], keys[1], keys[count-1]} {
		result := read("/v1/environments?include_archived=true&token="+tokenID+"&key="+key, "admin")
		if len(result.Items) != 1 || result.Items[0].Granted != (key == keys[1]) {
			t.Fatalf("grant state for %s: %#v", key, result)
		}
	}
	call("GET", "/v1/configs?limit=101", nil, "viewer", 400)
	call("GET", "/v1/environments?limit=1&limit=2", nil, "viewer", 400)
	call("GET", "/v1/vault-items?offset=-1", nil, "viewer", 400)
	call("POST", "/v1/api-tokens/"+tokenID+"/revoke", nil, "admin", 200)
	call("PATCH", patchPath, map[string]any{"add": []string{keys[0]}}, "admin", 422)
}
