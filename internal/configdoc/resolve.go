package configdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

const maxConfigBytes = 5 << 20

type Format string

const (
	YAML Format = "yaml"
	JSON Format = "json"
)

type Reference struct {
	NamespaceKey string
	ItemKey      string
	FieldKey     string
}

type Lookup func(Reference) (string, error)

var ErrTooLarge = errors.New("resolved config exceeds 5 MiB")

func Resolve(format Format, canonical []byte, lookup Lookup) ([]byte, error) {
	if len(canonical) > maxConfigBytes {
		return nil, ErrTooLarge
	}
	if lookup == nil {
		return nil, errors.New("Vault Reference lookup is required")
	}
	remaining := maxConfigBytes
	boundedLookup := func(reference Reference) (string, error) {
		value, err := lookup(reference)
		if err != nil {
			return "", err
		}
		if len(value) > remaining {
			return "", ErrTooLarge
		}
		remaining -= len(value)
		return value, nil
	}
	switch format {
	case YAML:
		return resolveYAML(canonical, boundedLookup)
	case JSON:
		return resolveJSON(canonical, boundedLookup)
	default:
		return nil, fmt.Errorf("unsupported Config format %q", format)
	}
}

func resolveJSON(canonical []byte, lookup Lookup) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder, 0)
	if err != nil {
		return nil, fmt.Errorf("parse canonical JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("canonical JSON contains trailing content")
		}
		return nil, fmt.Errorf("parse canonical JSON trailer: %w", err)
	}
	resolved, err := resolveJSONValue(value, lookup)
	if err != nil {
		return nil, err
	}
	output, err := encodeJSON(resolved)
	if err != nil {
		return nil, fmt.Errorf("encode resolved JSON: %w", err)
	}
	return output, nil
}

func decodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 100 {
		return nil, errors.New("JSON nesting exceeds 100 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		switch token.(type) {
		case nil, bool, string, json.Number:
			return token, nil
		default:
			return nil, errors.New("JSON contains an unsupported token")
		}
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("JSON contains duplicate key %q", key)
			}
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, errors.New("JSON object is not closed")
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, errors.New("JSON array is not closed")
		}
		return array, nil
	default:
		return nil, errors.New("JSON contains an unexpected delimiter")
	}
}

func resolveJSONValue(value any, lookup Lookup) (any, error) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			resolved, err := resolveJSONValue(child, lookup)
			if err != nil {
				return nil, err
			}
			current[key] = resolved
		}
		return current, nil
	case []any:
		for index, child := range current {
			resolved, err := resolveJSONValue(child, lookup)
			if err != nil {
				return nil, err
			}
			current[index] = resolved
		}
		return current, nil
	case string:
		reference, escaped, matched, err := parseReference(current)
		if err != nil {
			return nil, err
		}
		if escaped {
			return reference.String(), nil
		}
		if !matched {
			return current, nil
		}
		resolved, err := lookup(reference)
		if err != nil {
			return nil, fmt.Errorf("resolve Vault Reference %s: %w", reference.Path(), err)
		}
		return resolved, nil
	default:
		return value, nil
	}
}

func resolveYAML(canonical []byte, lookup Lookup) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(canonical))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse canonical YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("canonical YAML contains multiple documents")
		}
		return nil, fmt.Errorf("parse canonical YAML trailer: %w", err)
	}
	if err := resolveYAMLNode(&document, lookup, make(map[*yaml.Node]yamlVisitState)); err != nil {
		return nil, err
	}
	output, err := encodeYAML(&document)
	if err != nil {
		return nil, fmt.Errorf("encode resolved YAML: %w", err)
	}
	return output, nil
}

func resolveYAMLNode(node *yaml.Node, lookup Lookup, visits map[*yaml.Node]yamlVisitState) error {
	if node == nil {
		return nil
	}
	switch visits[node] {
	case yamlVisiting:
		return errors.New("canonical YAML contains an alias cycle")
	case yamlVisited:
		return nil
	}
	visits[node] = yamlVisiting
	defer func() { visits[node] = yamlVisited }()

	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveYAMLNode(child, lookup, visits); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return errors.New("canonical YAML contains an invalid mapping")
		}
		keys := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return errors.New("canonical YAML contains a non-scalar mapping key")
			}
			if _, duplicate := keys[key.Value]; duplicate {
				return fmt.Errorf("canonical YAML contains duplicate key %q", key.Value)
			}
			keys[key.Value] = struct{}{}
			if err := resolveYAMLNode(node.Content[index+1], lookup, visits); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return nil
		}
		reference, escaped, matched, err := parseReference(node.Value)
		if err != nil {
			return err
		}
		if escaped {
			node.Value = reference.String()
			return nil
		}
		if !matched {
			return nil
		}
		value, err := lookup(reference)
		if err != nil {
			return fmt.Errorf("resolve Vault Reference %s: %w", reference.Path(), err)
		}
		node.Tag = "!!str"
		node.Value = value
	case yaml.AliasNode:
		return resolveYAMLNode(node.Alias, lookup, visits)
	}
	return nil
}

func parseReference(value string) (Reference, bool, bool, error) {
	escaped := strings.HasPrefix(value, "{{vault.") && strings.HasSuffix(value, "}}")
	candidate := value
	if escaped {
		candidate = value[1 : len(value)-1]
	}
	if !strings.HasPrefix(candidate, "{vault.") {
		if strings.Contains(candidate, "{vault.") {
			return Reference{}, false, false, errors.New("Vault Reference must occupy the full scalar")
		}
		return Reference{}, false, false, nil
	}
	if !strings.HasSuffix(candidate, "}") {
		return Reference{}, false, false, errors.New("invalid Vault Reference")
	}
	parts := strings.Split(candidate[1:len(candidate)-1], ".")
	if len(parts) != 4 || parts[0] != "vault" || !validResourceKey(parts[1]) || !validResourceKey(parts[2]) || !validResourceKey(parts[3]) {
		return Reference{}, false, false, errors.New("invalid Vault Reference")
	}
	return Reference{NamespaceKey: parts[1], ItemKey: parts[2], FieldKey: parts[3]}, escaped, true, nil
}

func (reference Reference) Path() string {
	return reference.NamespaceKey + "." + reference.ItemKey + "." + reference.FieldKey
}

func (reference Reference) String() string {
	return "{vault." + reference.Path() + "}"
}

func validResourceKey(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}
