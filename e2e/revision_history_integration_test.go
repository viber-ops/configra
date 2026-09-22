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

func TestRevisionHistoryPagesThroughRealMySQLWithoutSkippingConcurrentWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store := newStore(t, ctx)
	handler := management.NewHandler(store, nil, nil)
	// Only the external identity adapter is supplied here. Mutations, reads,
	// pagination, encryption and storage run through the real HTTPS handler.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := humanauth.RoleAdmin
		if r.Method == http.MethodGet {
			role = humanauth.RoleViewer
		}
		handler.ServeHTTP(w, r.WithContext(humanauth.WithPrincipal(r.Context(), humanauth.Principal{Subject: "revision-pages", Role: role})))
	}))
	defer server.Close()
	client := server.Client()
	operation := 0
	call := func(method, path string, body any, wantStatus int) []byte {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", server.URL)
		operation++
		r.Header.Set("Idempotency-Key", fmt.Sprintf("revision-pages-%d", operation))
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err = io.ReadAll(response.Body)
		if err != nil || response.StatusCode != wantStatus {
			t.Fatalf("%s %s: status=%d, error=%v, body=%s", method, path, response.StatusCode, err, data)
		}
		return data
	}
	for _, key := range []string{"a", "b"} {
		call(http.MethodPost, "/v1/environments", map[string]any{"key": key, "display_name": key}, 200)
	}
	const configPath = "/v1/environments/a/configs/payment"
	const vaultPath = "/v1/vault-items/platform/redis"
	const secretMarker = "history-must-not-expose-this-secret"
	commitConfig := func(path string, revision int) {
		call(http.MethodPut, path, map[string]any{
			"name": "Payment", "expected_revision": revision - 1, "format": "yaml",
			"content": fmt.Sprintf("revision: %d\npassword: '%s'\n", revision, secretMarker),
		}, 200)
	}
	variantID := ""
	commitVault := func(revision int) {
		data := call(http.MethodPut, vaultPath, map[string]any{
			"display_name": "Redis", "expected_revision": revision - 1,
			"snapshot": vaultSnapshot(variantID, fmt.Sprintf("%s-%d", secretMarker, revision), "private-file-bytes"),
		}, 200)
		var result struct {
			VariantIDs []string `json:"variant_ids"`
		}
		if err := json.Unmarshal(data, &result); err != nil || len(result.VariantIDs) != 1 {
			t.Fatalf("Vault mutation result: %s, %v", data, err)
		}
		variantID = result.VariantIDs[0]
	}
	const revisions = 1005
	for revision := 1; revision <= revisions; revision++ {
		commitConfig(configPath, revision)
		commitVault(revision)
	}
	// The same Config in another Environment and the same Item in another
	// Namespace must not leak into either history.
	commitConfig("/v1/environments/b/configs/payment", 1)
	call(http.MethodPut, "/v1/vault-items/other/redis", map[string]any{
		"display_name": "Other Redis", "expected_revision": 0,
		"snapshot": vaultSnapshot("", "other-secret", "other-file"),
	}, 200)
	type page struct {
		Items []struct {
			Revision uint64 `json:"revision"`
		} `json:"items"`
		NextBefore uint64 `json:"next_before"`
	}
	maxRead := time.Duration(0)
	maxBytes := 0
	read := func(path, query string) page {
		t.Helper()
		start := time.Now()
		data := call(http.MethodGet, path+"/revisions"+query, nil, 200)
		maxRead = max(maxRead, time.Since(start))
		maxBytes = max(maxBytes, len(data))
		for _, forbidden := range []string{secretMarker, "private-file-bytes", `"content":`, `"snapshot":`, `"values":`} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("history contains private content %q", forbidden)
			}
		}
		var result page
		if err := json.Unmarshal(data, &result); err != nil || result.Items == nil {
			t.Fatalf("history must contain an items array: %s, %v", data, err)
		}
		return result
	}
	for _, path := range []string{configPath, vaultPath} {
		first := read(path, "")
		if len(first.Items) != 50 || first.Items[0].Revision != revisions || first.NextBefore != revisions-49 {
			t.Fatalf("default page: %#v", first)
		}
		if path == configPath {
			commitConfig(path, revisions+1)
		} else {
			commitVault(revisions + 1)
		}
		cursor := first.NextBefore
		wantRevision := cursor - 1
		for cursor != 0 {
			current := read(path, fmt.Sprintf("?limit=100&before=%d", cursor))
			if len(current.Items) == 0 || len(current.Items) > 100 {
				t.Fatalf("page size = %d", len(current.Items))
			}
			for _, item := range current.Items {
				if item.Revision != wantRevision {
					t.Fatalf("revision = %d, want %d after concurrent commit", item.Revision, wantRevision)
				}
				wantRevision--
			}
			cursor = current.NextBefore
			if wantRevision > 0 && cursor != current.Items[len(current.Items)-1].Revision {
				t.Fatalf("invalid continuation cursor: %#v", current)
			}
		}
		if wantRevision != 0 {
			t.Fatalf("missing %d older revisions", wantRevision)
		}
		if tail := read(path, "?limit=5&before=6"); len(tail.Items) != 5 || tail.NextBefore != 0 {
			t.Fatalf("exact-size last page must have no continuation: %#v", tail)
		}
		if empty := read(path, "?before=1"); len(empty.Items) != 0 || empty.NextBefore != 0 {
			t.Fatalf("past end is an empty successful page: %#v", empty)
		}
		if newest := read(path, "?limit=1&before=18446744073709551615"); len(newest.Items) != 1 || newest.Items[0].Revision != revisions+1 {
			t.Fatalf("new first page must see concurrent write: %#v", newest)
		}
	}
	for _, path := range []string{"/v1/environments/b/configs/payment", "/v1/vault-items/other/redis"} {
		if isolated := read(path, ""); len(isolated.Items) != 1 || isolated.Items[0].Revision != 1 || isolated.NextBefore != 0 {
			t.Fatalf("history identity was not isolated: %#v", isolated)
		}
	}
	call(http.MethodPost, "/v1/configs/payment/archive", nil, 200)
	call(http.MethodPost, vaultPath+"/archive", nil, 200)
	for _, path := range []string{configPath, vaultPath} {
		if archived := read(path, "?limit=1&before=2"); len(archived.Items) != 1 || archived.Items[0].Revision != 1 {
			t.Fatalf("archived history must remain readable: %#v", archived)
		}
	}
	for _, path := range []string{"/v1/environments/missing/configs/payment", "/v1/environments/a/configs/missing", "/v1/vault-items/platform/missing"} {
		call(http.MethodGet, path+"/revisions?before=1", nil, 404)
	}
	t.Logf("MySQL 8.0.22; %d Config and Vault revisions each; largest history response %d bytes; slowest HTTPS page %s (diagnostic, not a production load gate)", revisions+1, maxBytes, maxRead)
}
