package vaultimport

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/vaultdoc"
)

const example = `namespace: order_service
item: clients-conf
display_name: order_service_clients_conf
environments:
  - hk47
  - hk180
fields:
  - key: lvl-data
    name: LVL数据服务
    type: text
    value: "https://lvl.example.com/api/v1/lvl"
  - key: google-login
    type: secret
    value: "s3cr3t-value"
  - key: empty-allowed
    name: Empty
    type: text
    value: ""
`

func TestParseKeepsFileOrderAndDefaults(t *testing.T) {
	draft, problems := Parse([]byte(example))
	if len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
	if draft.NamespaceKey != "order_service" || draft.ItemKey != "clients-conf" || draft.DisplayName != "order_service_clients_conf" {
		t.Fatalf("identity = %+v", draft)
	}
	if !reflect.DeepEqual(draft.Environments, []string{"hk47", "hk180"}) {
		t.Fatalf("environments = %v", draft.Environments)
	}
	want := []vaultdoc.Field{
		{Key: "lvl-data", Name: "LVL数据服务", Type: vaultdoc.Text},
		{Key: "google-login", Name: "google-login", Type: vaultdoc.Secret},
		{Key: "empty-allowed", Name: "Empty", Type: vaultdoc.Text},
	}
	if !reflect.DeepEqual(draft.Fields, want) {
		t.Fatalf("fields = %+v", draft.Fields)
	}
	if draft.Values["lvl-data"] != "https://lvl.example.com/api/v1/lvl" || draft.Values["google-login"] != "s3cr3t-value" || draft.Values["empty-allowed"] != "" || len(draft.Values) != 3 {
		t.Fatalf("values = %v", draft.Values)
	}
	if draft.Line("environments[1]") != 6 || draft.Line("item") != 2 {
		t.Fatalf("lines = %d, %d", draft.Line("environments[1]"), draft.Line("item"))
	}
}

func TestParseDefaultsTheDisplayNameToTheItemKey(t *testing.T) {
	draft, problems := Parse([]byte("namespace: a\nitem: b\nenvironments: [x]\nfields:\n  - {key: k, type: text, value: v}\n"))
	if len(problems) != 0 || draft.DisplayName != "b" {
		t.Fatalf("draft = %+v, problems = %+v", draft, problems)
	}
}

func TestParseNineteenFields(t *testing.T) {
	var source strings.Builder
	source.WriteString("namespace: order_service\nitem: clients-conf\nenvironments: [hk47]\nfields:\n")
	for index := range 19 {
		fmt.Fprintf(&source, "  - key: service-%02d\n    type: text\n    value: \"https://service-%02d.example.com/api\"\n", 18-index, 18-index)
	}
	draft, problems := Parse([]byte(source.String()))
	if len(problems) != 0 || len(draft.Fields) != 19 || draft.Fields[0].Key != "service-18" || draft.Fields[18].Key != "service-00" {
		t.Fatalf("draft = %+v, problems = %+v", draft.Fields, problems)
	}
}

func TestParseReportsProblemsByPathAndLineWithoutValues(t *testing.T) {
	cases := map[string]struct {
		source string
		want   []Problem
	}{
		"syntax": {
			source: "namespace: a\nitem: b: c\n",
			want:   []Problem{{Line: 2, Code: CodeSyntax}},
		},
		"empty": {source: "  \n# nothing\n", want: []Problem{{Code: CodeEmpty}}},
		"two documents": {
			source: "namespace: a\n---\nnamespace: b\n",
			want:   []Problem{{Line: 2, Code: CodeMultipleDocuments}},
		},
		"not a mapping": {source: "- a\n", want: []Problem{{Line: 1, Code: CodeExpectedMapping}}},
		"unknown and repeated keys": {
			source: "namespace: a\nitem: b\nitem: c\ncolour: red\nenvironments: [x]\nfields:\n  - {key: k, type: text, value: v, secret: true}\n",
			want: []Problem{
				{Path: "item", Line: 3, Code: CodeDuplicateKey},
				{Path: "colour", Line: 4, Code: CodeUnknownKey},
				{Path: "fields[0].secret", Line: 7, Code: CodeUnknownKey},
			},
		},
		"missing, null and non-string values": {
			source: "namespace: a\nitem: b\nenvironments: [x]\nfields:\n  - key: one\n    type: text\n  - key: two\n    type: text\n    value:\n  - key: three\n    type: text\n    value: 6379\n",
			want: []Problem{
				{Path: "fields[0].value", Line: 5, Code: CodeMissing},
				{Path: "fields[1].value", Line: 9, Code: CodeMissing},
				{Path: "fields[2].value", Line: 12, Code: CodeExpectedString},
			},
		},
		"identifiers, names and types": {
			source: "namespace: Order\nitem: b\ndisplay_name: \"\"\nenvironments: [x]\nfields:\n  - {key: 9lives, type: text, value: v}\n  - {key: f, type: file, value: v}\n  - {key: g, type: number, value: v}\n  - {key: h, name: \"\", type: secret, value: v}\n",
			want: []Problem{
				{Path: "namespace", Line: 1, Code: CodeInvalidKey},
				{Path: "display_name", Line: 3, Code: CodeInvalidName},
				{Path: "fields[0].key", Line: 6, Code: CodeInvalidKey},
				{Path: "fields[1].type", Line: 7, Code: CodeFileType},
				{Path: "fields[2].type", Line: 8, Code: CodeUnsupportedType},
				{Path: "fields[3].name", Line: 9, Code: CodeInvalidName},
			},
		},
		"duplicates in lists": {
			source: "namespace: a\nitem: b\nenvironments: [x, x]\nfields:\n  - {key: k, type: text, value: v}\n  - {key: k, type: secret, value: w}\n",
			want: []Problem{
				{Path: "environments[1]", Line: 3, Code: CodeDuplicateEnvironment},
				{Path: "fields[1].key", Line: 6, Code: CodeDuplicateField},
			},
		},
		"empty and missing lists": {
			source: "namespace: a\nitem: b\nenvironments: []\n",
			want: []Problem{
				{Path: "environments", Line: 3, Code: CodeEmptyList},
				{Path: "fields", Line: 1, Code: CodeMissing},
			},
		},
		"aliases": {
			source: "namespace: &n a\nitem: *n\nenvironments: [x]\nfields:\n  - {key: k, type: text, value: v}\n",
			want:   []Problem{{Path: "item", Line: 2, Code: CodeAlias}},
		},
		"oversized value": {
			source: "namespace: a\nitem: b\nenvironments: [x]\nfields:\n  - key: k\n    type: secret\n    value: " + strings.Repeat("x", vaultdoc.MaxTextBytes+1) + "\n",
			want:   []Problem{{Path: "fields[0].value", Line: 7, Code: CodeValueTooLarge}},
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			draft, problems := Parse([]byte(test.source))
			if !reflect.DeepEqual(problems, test.want) {
				t.Fatalf("problems = %+v, want %+v", problems, test.want)
			}
			if !reflect.DeepEqual(draft, Draft{}) {
				t.Fatalf("a file with problems returned a draft: %+v", draft)
			}
		})
	}
}

func TestParseBoundsTheFileAndTheReport(t *testing.T) {
	if _, problems := Parse(make([]byte, MaxBytes+1)); !reflect.DeepEqual(problems, []Problem{{Code: CodeTooLarge}}) {
		t.Fatalf("oversized file problems = %+v", problems)
	}
	var source strings.Builder
	source.WriteString("namespace: a\nitem: b\nenvironments: [x]\nfields:\n")
	for range 300 {
		source.WriteString("  - {key: Bad, type: text}\n")
	}
	if _, problems := Parse([]byte(source.String())); len(problems) != MaxProblems {
		t.Fatalf("report has %d problems, want %d", len(problems), MaxProblems)
	}
}
