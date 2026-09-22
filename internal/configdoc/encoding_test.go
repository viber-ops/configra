package configdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func FuzzJSONBudgetMatchesEncoding(f *testing.F) {
	for _, source := range []string{
		`null`, `true`, `1.20`, `[]`, `{}`, `"<>&\u2028\u2029\u0000\n\"\\"`,
		`{"a":{},"b":[1,{"c":[false,null,"中文"]}]}`, "\"\xff\"",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		decoder := json.NewDecoder(strings.NewReader(source))
		decoder.UseNumber()
		value, err := decodeJSONValue(decoder, 0)
		if err != nil {
			return
		}
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		remaining := len(encoded)
		if err := checkJSONBudget(value, 0, &remaining); err != nil || remaining != 0 {
			t.Fatalf("exact encoding budget: remaining=%d error=%v", remaining, err)
		}
		remaining = len(encoded) - 1
		if err := checkJSONBudget(value, 0, &remaining); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("one byte short: %v", err)
		}
	})
}

func TestEncodersPreserveTheFiveMiBBoundary(t *testing.T) {
	for _, format := range []Format{JSON, YAML} {
		t.Run(string(format), func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				value := strings.Repeat("x", maxConfigBytes-3+extra) // Quotes and final newline.
				var content []byte
				var err error
				if format == JSON {
					content, err = encodeJSON(value)
				} else {
					content, err = encodeYAML(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: value})
				}
				if extra == 0 {
					if err != nil || len(content) != maxConfigBytes {
						t.Fatalf("valid boundary: bytes=%d error=%v", len(content), err)
					}
				} else if !errors.Is(err, ErrTooLarge) || len(content) != 0 {
					t.Fatalf("oversized boundary: bytes=%d error=%v", len(content), err)
				}
			}
		})
	}
}

func TestJSONBudgetHandlesNilCollections(t *testing.T) {
	for _, value := range []any{map[string]any(nil), []any(nil)} {
		remaining := 4
		if err := checkJSONBudget(value, 0, &remaining); err != nil || remaining != 0 {
			t.Fatalf("null budget: remaining=%d error=%v", remaining, err)
		}
	}
}

func TestConfigBufferRejectsAnOversizedWriteBeforeGrowing(t *testing.T) {
	var output configBuffer
	if n, err := output.Write(bytes.Repeat([]byte("x"), maxConfigBytes-1)); err != nil || n != maxConfigBytes-1 {
		t.Fatalf("initial write: bytes=%d error=%v", n, err)
	}
	if n, err := output.Write([]byte("xx")); n != 0 || !errors.Is(err, ErrTooLarge) || !output.exceeded || output.Len() != maxConfigBytes-1 {
		t.Fatalf("overflow must not grow the buffer: bytes=%d len=%d error=%v", n, output.Len(), err)
	}
}
