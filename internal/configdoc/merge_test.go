package configdoc_test

import (
	"bytes"
	"testing"

	"github.com/viber-ops/configra/internal/configdoc"
)

func TestMergeOverlaysSourceMappingsAndKeepsTargetOnlyKeys(t *testing.T) {
	tests := []struct {
		name   string
		format configdoc.Format
		source string
		target string
		want   string
	}{
		{
			name: "YAML", format: configdoc.YAML,
			source: "database:\n  host: source\n  ports: [2]\nsource_only: true\n",
			target: "# target\ndatabase:\n  host: target\n  ports: [1]\n  target_only: keep\ntarget_only: yes\n",
			want:   "# target\ndatabase:\n  host: source\n  ports: [2]\n  target_only: keep\ntarget_only: yes\nsource_only: true\n",
		},
		{
			name: "JSON", format: configdoc.JSON,
			source: `{"database":{"host":"source","ports":[2]},"source_only":true}`,
			target: `{"database":{"host":"target","ports":[1],"target_only":"keep"},"target_only":true}`,
			want:   "{\n  \"database\": {\n    \"host\": \"source\",\n    \"ports\": [\n      2\n    ],\n    \"target_only\": \"keep\"\n  },\n  \"source_only\": true,\n  \"target_only\": true\n}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merged, err := configdoc.Merge(test.format, []byte(test.source), []byte(test.target))
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			if string(merged.Content) != test.want {
				t.Fatalf("merged =\n%s\nwant:\n%s", merged.Content, test.want)
			}
		})
	}
}

func TestMergeTreatsSourceNullAsAValueAndLeavesInputsUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		format configdoc.Format
		source string
		target string
		want   string
	}{
		{
			name: "YAML", format: configdoc.YAML,
			source: "value: null\nnested:\n  replaced: null\n",
			target: "value: keep\nnested:\n  replaced: keep\n  target_only: true\n",
			want:   "value: null\nnested:\n  replaced: null\n  target_only: true\n",
		},
		{
			name: "JSON", format: configdoc.JSON,
			source: `{"value":null,"nested":{"replaced":null}}`,
			target: `{"value":"keep","nested":{"replaced":"keep","target_only":true}}`,
			want:   "{\n  \"nested\": {\n    \"replaced\": null,\n    \"target_only\": true\n  },\n  \"value\": null\n}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			target := []byte(test.target)
			sourceBefore := bytes.Clone(source)
			targetBefore := bytes.Clone(target)
			merged, err := configdoc.Merge(test.format, source, target)
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			if string(merged.Content) != test.want {
				t.Fatalf("merged =\n%s\nwant:\n%s", merged.Content, test.want)
			}
			if !bytes.Equal(source, sourceBefore) || !bytes.Equal(target, targetBefore) {
				t.Fatal("Merge mutated an input buffer")
			}
		})
	}
}

func TestMergeRejectsInvalidSourceOrTarget(t *testing.T) {
	if _, err := configdoc.Merge(configdoc.YAML, []byte("a: [\n"), []byte("a: 1\n")); err == nil {
		t.Fatal("Merge accepted invalid Source")
	}
	if _, err := configdoc.Merge(configdoc.JSON, []byte(`{"a":1}`), []byte(`{"a":1,"a":2}`)); err == nil {
		t.Fatal("Merge accepted invalid Target")
	}
}
