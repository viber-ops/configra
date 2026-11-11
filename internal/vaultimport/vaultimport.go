// Package vaultimport reads the YAML file an administrator uses to create one Vault Item with one Variant (#55).
//
// The file is a description of an Item, not a Vault value:
//
//	namespace: order_service
//	item: clients-conf
//	display_name: order_service_clients_conf   # optional, defaults to item
//	environments: [hk47]
//	fields:
//	  - key: lvl-data
//	    name: LVL data                         # optional, defaults to key
//	    type: text                             # text or secret
//	    value: "https://lvl.example.com/api/v1/lvl"
//
// Parsing is strict: unknown and repeated keys, aliases, non-string values and a missing value are problems, never
// silently ignored or converted. A Problem names a path and line and never carries a value.
package vaultimport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/viber-ops/configra/internal/vaultdoc"
)

const (
	// MaxBytes bounds the import file. It is far above a few hundred URL Fields and keeps parsing memory small.
	MaxBytes        = 1 << 20
	MaxFields       = 1000
	MaxEnvironments = 256
	// MaxProblems keeps a badly broken file from producing an unreadable report.
	MaxProblems  = 100
	maxNameBytes = 255
)

// Problem codes. The Management UI translates them; they are part of the preview response.
const (
	CodeTooLarge             = "too_large"
	CodeSyntax               = "syntax"
	CodeEmpty                = "empty"
	CodeMultipleDocuments    = "multiple_documents"
	CodeAlias                = "alias"
	CodeExpectedMapping      = "expected_mapping"
	CodeExpectedList         = "expected_list"
	CodeExpectedString       = "expected_string"
	CodeUnknownKey           = "unknown_key"
	CodeDuplicateKey         = "duplicate_key"
	CodeMissing              = "missing"
	CodeInvalidKey           = "invalid_key"
	CodeInvalidName          = "invalid_name"
	CodeUnsupportedType      = "unsupported_type"
	CodeFileType             = "file_type"
	CodeDuplicateField       = "duplicate_field"
	CodeDuplicateEnvironment = "duplicate_environment"
	CodeEmptyList            = "empty_list"
	CodeTooMany              = "too_many"
	CodeValueTooLarge        = "value_too_large"
	CodeEnvironmentUnknown   = "environment_unknown"
	CodeEnvironmentArchived  = "environment_archived"
	CodeItemExists           = "item_exists"
	CodeItemArchived         = "item_archived"
)

type Problem struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Code string `json:"code"`
}

// Draft is a valid import, in file order. Values are keyed by Field key.
type Draft struct {
	NamespaceKey string            `json:"namespace_key"`
	ItemKey      string            `json:"item_key"`
	DisplayName  string            `json:"display_name"`
	Environments []string          `json:"environments"`
	Fields       []vaultdoc.Field  `json:"fields"`
	Values       map[string]string `json:"values"`
	lines        map[string]int
}

// Line is the line of a path in the imported file, or 0.
func (draft Draft) Line(path string) int {
	return draft.lines[path]
}

var syntaxLine = regexp.MustCompile(`line (\d+)`)

// Parse reads one import file. It returns a Draft only when there are no problems.
func Parse(content []byte) (Draft, []Problem) {
	if len(content) > MaxBytes {
		return Draft{}, []Problem{{Code: CodeTooLarge}}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return Draft{}, []Problem{{Code: CodeEmpty}}
		}
		// The parser's message can quote the document, so only its line is kept.
		problem := Problem{Code: CodeSyntax}
		if match := syntaxLine.FindStringSubmatch(err.Error()); match != nil {
			problem.Line, _ = strconv.Atoi(match[1])
		}
		return Draft{}, []Problem{problem}
	}
	var next yaml.Node
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		return Draft{}, []Problem{{Code: CodeMultipleDocuments, Line: next.Line}}
	}
	if len(document.Content) == 0 {
		return Draft{}, []Problem{{Code: CodeEmpty}}
	}
	reader := &reader{lines: map[string]int{}}
	draft := reader.item(document.Content[0])
	if len(reader.problems) > 0 {
		return Draft{}, reader.problems
	}
	draft.lines = reader.lines
	return draft, nil
}

type reader struct {
	problems []Problem
	lines    map[string]int
}

func (reader *reader) add(path string, line int, code string) {
	if len(reader.problems) >= MaxProblems {
		return
	}
	reader.problems = append(reader.problems, Problem{Path: path, Line: line, Code: code})
}

type entry struct {
	key   *yaml.Node
	value *yaml.Node
}

// mapping returns a mapping's entries by key, reporting keys outside allowed and repeated keys.
func (reader *reader) mapping(node *yaml.Node, path string, allowed ...string) (map[string]entry, bool) {
	if node.Kind == yaml.AliasNode {
		reader.add(path, node.Line, CodeAlias)
		return nil, false
	}
	if node.Kind != yaml.MappingNode {
		reader.add(path, node.Line, CodeExpectedMapping)
		return nil, false
	}
	entries := make(map[string]entry, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		name := key.Value
		child := join(path, name)
		switch {
		case key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" || !slices.Contains(allowed, name):
			reader.add(child, key.Line, CodeUnknownKey)
		case entries[name].key != nil:
			reader.add(child, key.Line, CodeDuplicateKey)
		default:
			entries[name] = entry{key: key, value: value}
		}
	}
	return entries, true
}

// text reads a string scalar. A missing or null value is reported as missing when required.
func (reader *reader) text(parent *yaml.Node, entries map[string]entry, path, name string, required bool) (string, bool) {
	child := join(path, name)
	found, ok := entries[name]
	if !ok || (found.value.Kind == yaml.ScalarNode && found.value.ShortTag() == "!!null") {
		if required {
			line := parent.Line
			if ok {
				line = found.key.Line
			}
			reader.add(child, line, CodeMissing)
		}
		return "", false
	}
	reader.lines[child] = found.value.Line
	switch {
	case found.value.Kind == yaml.AliasNode:
		reader.add(child, found.value.Line, CodeAlias)
	case found.value.Kind != yaml.ScalarNode || found.value.ShortTag() != "!!str":
		reader.add(child, found.value.Line, CodeExpectedString)
	default:
		return found.value.Value, true
	}
	return "", false
}

func (reader *reader) key(parent *yaml.Node, entries map[string]entry, path, name string) string {
	value, ok := reader.text(parent, entries, path, name, true)
	if ok && !vaultdoc.ValidResourceKey(value) {
		reader.add(join(path, name), reader.lines[join(path, name)], CodeInvalidKey)
		return ""
	}
	return value
}

func (reader *reader) name(parent *yaml.Node, entries map[string]entry, path, name, fallback string) string {
	value, ok := reader.text(parent, entries, path, name, false)
	if !ok {
		return fallback
	}
	if value == "" || len(value) > maxNameBytes {
		reader.add(join(path, name), reader.lines[join(path, name)], CodeInvalidName)
	}
	return value
}

func (reader *reader) list(parent *yaml.Node, entries map[string]entry, name string, maximum int) []*yaml.Node {
	found, ok := entries[name]
	switch {
	case !ok || (found.value.Kind == yaml.ScalarNode && found.value.ShortTag() == "!!null"):
		line := parent.Line
		if ok {
			line = found.key.Line
		}
		reader.add(name, line, CodeMissing)
	case found.value.Kind == yaml.AliasNode:
		reader.add(name, found.value.Line, CodeAlias)
	case found.value.Kind != yaml.SequenceNode:
		reader.add(name, found.value.Line, CodeExpectedList)
	case len(found.value.Content) == 0:
		reader.add(name, found.value.Line, CodeEmptyList)
	case len(found.value.Content) > maximum:
		reader.add(name, found.value.Line, CodeTooMany)
	default:
		return found.value.Content
	}
	return nil
}

func (reader *reader) item(root *yaml.Node) Draft {
	entries, ok := reader.mapping(root, "", "namespace", "item", "display_name", "environments", "fields")
	if !ok {
		return Draft{}
	}
	draft := Draft{Values: map[string]string{}}
	draft.NamespaceKey = reader.key(root, entries, "", "namespace")
	draft.ItemKey = reader.key(root, entries, "", "item")
	draft.DisplayName = reader.name(root, entries, "", "display_name", draft.ItemKey)

	seenEnvironments := map[string]bool{}
	for index, node := range reader.list(root, entries, "environments", MaxEnvironments) {
		path := fmt.Sprintf("environments[%d]", index)
		reader.lines[path] = node.Line
		switch {
		case node.Kind == yaml.AliasNode:
			reader.add(path, node.Line, CodeAlias)
		case node.Kind != yaml.ScalarNode || node.ShortTag() != "!!str":
			reader.add(path, node.Line, CodeExpectedString)
		case !vaultdoc.ValidResourceKey(node.Value):
			reader.add(path, node.Line, CodeInvalidKey)
		case seenEnvironments[node.Value]:
			reader.add(path, node.Line, CodeDuplicateEnvironment)
		default:
			seenEnvironments[node.Value] = true
			draft.Environments = append(draft.Environments, node.Value)
		}
	}

	seenFields := map[string]bool{}
	for index, node := range reader.list(root, entries, "fields", MaxFields) {
		path := fmt.Sprintf("fields[%d]", index)
		fieldEntries, ok := reader.mapping(node, path, "key", "name", "type", "value")
		if !ok {
			continue
		}
		key := reader.key(node, fieldEntries, path, "key")
		name := reader.name(node, fieldEntries, path, "name", key)
		fieldType, typed := reader.text(node, fieldEntries, path, "type", true)
		switch {
		case !typed:
		case fieldType == string(vaultdoc.File):
			reader.add(path+".type", reader.lines[path+".type"], CodeFileType)
		case fieldType != string(vaultdoc.Text) && fieldType != string(vaultdoc.Secret):
			reader.add(path+".type", reader.lines[path+".type"], CodeUnsupportedType)
		}
		value, valued := reader.text(node, fieldEntries, path, "value", true)
		if fieldType == string(vaultdoc.Text) {
			// The draft shows the value the create form will store (#62).
			value = vaultdoc.NormalizeText(value)
		}
		if valued && len(value) > vaultdoc.MaxTextBytes {
			reader.add(path+".value", reader.lines[path+".value"], CodeValueTooLarge)
		}
		if key == "" {
			continue
		}
		if seenFields[key] {
			reader.add(path+".key", reader.lines[path+".key"], CodeDuplicateField)
			continue
		}
		seenFields[key] = true
		draft.Fields = append(draft.Fields, vaultdoc.Field{Key: key, Name: name, Type: vaultdoc.FieldType(fieldType)})
		draft.Values[key] = value
	}
	return draft
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
