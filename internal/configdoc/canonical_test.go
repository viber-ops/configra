package configdoc_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/configdoc"
)

func TestCanonicalizeYAMLFormatsOnceAndFindsUniqueVaultReferences(t *testing.T) {
	source := []byte("# database settings\ndatabase:\n    password: \"{vault.platform.redis.password}\"\n    replica: \"{vault.platform.redis.password}\"\nliteral: \"{{vault.platform.redis.password}}\"\n")
	document, err := configdoc.Canonicalize(configdoc.YAML, source)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	const expected = "# database settings\ndatabase:\n  password: \"{vault.platform.redis.password}\"\n  replica: \"{vault.platform.redis.password}\"\nliteral: \"{{vault.platform.redis.password}}\"\n"
	if string(document.Content) != expected {
		t.Fatalf("canonical YAML =\n%s\nwant:\n%s", document.Content, expected)
	}
	if len(document.References) != 1 || document.References[0] != (configdoc.Reference{NamespaceKey: "platform", ItemKey: "redis", FieldKey: "password"}) {
		t.Fatalf("References = %#v", document.References)
	}
	again, err := configdoc.Canonicalize(configdoc.YAML, document.Content)
	if err != nil || !bytes.Equal(again.Content, document.Content) {
		t.Fatalf("second Canonicalize = %q, %v", again.Content, err)
	}
}

func TestCanonicalizeJSONIsDeterministicAndPreservesNumberLexemes(t *testing.T) {
	document, err := configdoc.Canonicalize(configdoc.JSON, []byte(`{"z":1.20,"a":{"password":"{vault.platform.redis.password}"}}`))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	const expected = "{\n  \"a\": {\n    \"password\": \"{vault.platform.redis.password}\"\n  },\n  \"z\": 1.20\n}\n"
	if string(document.Content) != expected {
		t.Fatalf("canonical JSON =\n%s\nwant:\n%s", document.Content, expected)
	}
	if len(document.References) != 1 || document.References[0].ItemKey != "redis" {
		t.Fatalf("References = %#v", document.References)
	}
}

func TestCanonicalizeRejectsInvalidDocumentsAndReferences(t *testing.T) {
	deepJSON := strings.Repeat("[", 102) + "0" + strings.Repeat("]", 102)
	tests := []struct {
		name   string
		format configdoc.Format
		source []byte
	}{
		{"duplicate YAML key", configdoc.YAML, []byte("a: 1\na: 2\n")},
		{"multiple YAML documents", configdoc.YAML, []byte("a: 1\n---\nb: 2\n")},
		{"invalid YAML", configdoc.YAML, []byte("a: [\n")},
		{"YAML alias cycle", configdoc.YAML, []byte("a: &cycle [*cycle]\n")},
		{"partial YAML reference", configdoc.YAML, []byte("a: prefix-{vault.platform.redis.password}\n")},
		{"legacy three-part YAML reference", configdoc.YAML, []byte("a: \"{vault.redis.password}\"\n")},
		{"duplicate JSON key", configdoc.JSON, []byte(`{"a":1,"a":2}`)},
		{"trailing JSON", configdoc.JSON, []byte(`{"a":1} {"b":2}`)},
		{"deep JSON", configdoc.JSON, []byte(deepJSON)},
		{"invalid JSON reference", configdoc.JSON, []byte(`{"a":"{vault.Platform.redis.password}"}`)},
		{"unknown format", configdoc.Format("toml"), []byte("a = 1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := configdoc.Canonicalize(test.format, test.source); err == nil {
				t.Fatal("Canonicalize accepted invalid input")
			}
		})
	}
	if _, err := configdoc.Canonicalize(configdoc.YAML, bytes.Repeat([]byte("a"), (5<<20)+1)); !errors.Is(err, configdoc.ErrTooLarge) {
		t.Fatalf("oversized Config error = %v, want ErrTooLarge", err)
	}
}
