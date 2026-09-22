package configdoc_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/configdoc"
	"go.yaml.in/yaml/v3"
)

func TestResolveStopsBeforeExpandingAnOversizedDocument(t *testing.T) {
	value := strings.Repeat("x", 512<<10)
	for _, format := range []configdoc.Format{configdoc.JSON, configdoc.YAML} {
		t.Run(string(format), func(t *testing.T) {
			// A small valid document would expand to 8 MiB. Stop the old code's
			// lookup before encoding it so this regression never needs a huge buffer.
			source := []byte("[" + strings.Repeat(`"{vault.platform.redis.password}",`, 15) + `"{vault.platform.redis.password}"]`)
			calls := 0
			content, err := configdoc.Resolve(format, source, func(configdoc.Reference) (string, error) {
				calls++
				if calls > 11 {
					return "", errors.New("lookup continued past the document budget")
				}
				return value, nil
			})
			if !errors.Is(err, configdoc.ErrTooLarge) || len(content) != 0 || calls != 11 {
				t.Fatalf("oversized expansion: calls=%d bytes=%d error=%v; want an early, value-free size error", calls, len(content), err)
			}
		})
	}
}

func BenchmarkResolveOversizedReferences(b *testing.B) {
	value := strings.Repeat("x", 512<<10)
	source := []byte("[" + strings.Repeat(`"{vault.platform.redis.password}",`, 15) + `"{vault.platform.redis.password}"]`)
	for _, format := range []configdoc.Format{configdoc.JSON, configdoc.YAML} {
		b.Run(string(format), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				content, err := configdoc.Resolve(format, source, func(configdoc.Reference) (string, error) { return value, nil })
				if !errors.Is(err, configdoc.ErrTooLarge) || len(content) != 0 {
					b.Fatalf("oversized document: bytes=%d error=%v", len(content), err)
				}
			}
		})
	}
}

func TestResolveYAMLReplacesFullScalarVaultReferencesAndPreservesEscapes(t *testing.T) {
	canonical := []byte("# application database\ndatabase:\n  username: \"{vault.platform.redis.username}\"\n  password: \"{vault.platform.redis.password}\"\nliteral: \"{{vault.platform.redis.password}}\"\n")
	values := map[configdoc.Reference]string{
		{NamespaceKey: "platform", ItemKey: "redis", FieldKey: "username"}: "redis-user",
		{NamespaceKey: "platform", ItemKey: "redis", FieldKey: "password"}: "secret:with-yaml-syntax",
	}

	resolved, err := configdoc.Resolve(configdoc.YAML, canonical, func(reference configdoc.Reference) (string, error) {
		value, ok := values[reference]
		if !ok {
			return "", fmt.Errorf("missing %s.%s", reference.ItemKey, reference.FieldKey)
		}
		return value, nil
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	const expected = "# application database\ndatabase:\n  username: \"redis-user\"\n  password: \"secret:with-yaml-syntax\"\nliteral: \"{vault.platform.redis.password}\"\n"
	if string(resolved) != expected {
		t.Fatalf("resolved YAML =\n%s\nwant:\n%s", resolved, expected)
	}
}

func TestResolveJSONReplacesFullScalarVaultReferencesCanonically(t *testing.T) {
	canonical := []byte(`{"database":{"password":"{vault.platform.redis.password}","port":6379},"literal":"{{vault.platform.redis.password}}"}`)
	resolved, err := configdoc.Resolve(configdoc.JSON, canonical, func(reference configdoc.Reference) (string, error) {
		if reference != (configdoc.Reference{NamespaceKey: "platform", ItemKey: "redis", FieldKey: "password"}) {
			return "", fmt.Errorf("unexpected reference %#v", reference)
		}
		return "secret:json-value", nil
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	const expected = "{\n  \"database\": {\n    \"password\": \"secret:json-value\",\n    \"port\": 6379\n  },\n  \"literal\": \"{vault.platform.redis.password}\"\n}\n"
	if string(resolved) != expected {
		t.Fatalf("resolved JSON =\n%s\nwant:\n%s", resolved, expected)
	}
}

func TestResolveYAMLAliasesDoNotResolveLiteralValuesAgain(t *testing.T) {
	tests := []struct {
		name      string
		scalar    string
		value     string
		want      string
		wantCalls int
	}{
		{
			name:   "escaped reference",
			scalar: "{{vault.platform.redis.password}}",
			want:   "{vault.platform.redis.password}",
		},
		{
			name:      "reference-shaped secret",
			scalar:    "{vault.platform.redis.password}",
			value:     "{vault.platform.mysql.password}",
			want:      "{vault.platform.mysql.password}",
			wantCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(fmt.Sprintf("first: &shared %q\nsecond: *shared\nthird: *shared\n", test.scalar))
			document, err := configdoc.Canonicalize(configdoc.YAML, source)
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			calls := 0
			resolved, err := configdoc.Resolve(configdoc.YAML, document.Content, func(reference configdoc.Reference) (string, error) {
				calls++
				if test.wantCalls == 0 || reference.Path() != "platform.redis.password" {
					return "", fmt.Errorf("unexpected lookup of %s", reference.Path())
				}
				return test.value, nil
			})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if calls != test.wantCalls {
				t.Fatalf("lookup calls = %d, want %d", calls, test.wantCalls)
			}
			var values map[string]string
			if err := yaml.Unmarshal(resolved, &values); err != nil {
				t.Fatalf("decode resolved YAML: %v", err)
			}
			for _, key := range []string{"first", "second", "third"} {
				if values[key] != test.want {
					t.Errorf("%s = %q, want %q", key, values[key], test.want)
				}
			}
		})
	}
}

func TestYAMLSharedAliasGraph(t *testing.T) {
	var source strings.Builder
	source.WriteString("base: &level0 \"{vault.platform.redis.password}\"\n")
	for level := 1; level <= 48; level++ {
		fmt.Fprintf(&source, "level%d: &level%d [*level%d, *level%d]\n", level, level, level-1, level-1)
	}
	document, err := configdoc.Canonicalize(configdoc.YAML, []byte(source.String()))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if len(document.References) != 1 || document.References[0].Path() != "platform.redis.password" {
		t.Fatalf("References = %#v", document.References)
	}
	calls := 0
	resolved, err := configdoc.Resolve(configdoc.YAML, document.Content, func(configdoc.Reference) (string, error) {
		calls++
		return "resolved-value", nil
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if calls != 1 || !strings.Contains(string(resolved), `base: &level0 "resolved-value"`) {
		t.Fatalf("shared reference was not resolved once: calls = %d", calls)
	}
}

func TestResolveYAMLRejectsAliasCycle(t *testing.T) {
	_, err := configdoc.Resolve(configdoc.YAML, []byte("a: &cycle [*cycle]\n"), func(configdoc.Reference) (string, error) {
		t.Fatal("unexpected lookup")
		return "", nil
	})
	if err == nil {
		t.Fatal("Resolve accepted an alias cycle")
	}
}
