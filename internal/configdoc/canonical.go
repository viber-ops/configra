package configdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"go.yaml.in/yaml/v3"
)

type Document struct {
	Content    []byte
	References []Reference
}

type yamlVisitState uint8

const (
	yamlVisiting yamlVisitState = iota + 1
	yamlVisited
)

func Canonicalize(format Format, source []byte) (Document, error) {
	if len(source) > maxConfigBytes {
		return Document{}, ErrTooLarge
	}
	switch format {
	case YAML:
		return canonicalizeYAML(source)
	case JSON:
		return canonicalizeJSON(source)
	default:
		return Document{}, fmt.Errorf("unsupported Config format %q", format)
	}
}

func canonicalizeJSON(source []byte) (Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder, 0)
	if err != nil {
		return Document{}, fmt.Errorf("parse JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("JSON contains trailing content")
		}
		return Document{}, fmt.Errorf("parse JSON trailer: %w", err)
	}
	references := make(map[Reference]struct{})
	if err := findJSONReferences(value, references); err != nil {
		return Document{}, err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Document{}, fmt.Errorf("format JSON: %w", err)
	}
	content = append(content, '\n')
	if len(content) > maxConfigBytes {
		return Document{}, ErrTooLarge
	}
	return Document{Content: content, References: sortedReferences(references)}, nil
}

func findJSONReferences(value any, references map[Reference]struct{}) error {
	switch current := value.(type) {
	case map[string]any:
		for _, child := range current {
			if err := findJSONReferences(child, references); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := findJSONReferences(child, references); err != nil {
				return err
			}
		}
	case string:
		reference, escaped, matched, err := parseReference(current)
		if err != nil {
			return err
		}
		if matched && !escaped {
			references[reference] = struct{}{}
		}
	}
	return nil
}

func canonicalizeYAML(source []byte) (Document, error) {
	document, references, err := parseYAML(source)
	if err != nil {
		return Document{}, err
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return Document{}, fmt.Errorf("format YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return Document{}, fmt.Errorf("close YAML formatter: %w", err)
	}
	if output.Len() > maxConfigBytes {
		return Document{}, ErrTooLarge
	}
	if _, _, err := parseYAML(output.Bytes()); err != nil {
		return Document{}, fmt.Errorf("validate formatted YAML: %w", err)
	}
	return Document{Content: output.Bytes(), References: sortedReferences(references)}, nil
}

func parseYAML(source []byte) (yaml.Node, map[Reference]struct{}, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return yaml.Node{}, nil, fmt.Errorf("parse YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return yaml.Node{}, nil, errors.New("YAML contains multiple documents")
		}
		return yaml.Node{}, nil, fmt.Errorf("parse YAML trailer: %w", err)
	}
	references := make(map[Reference]struct{})
	if err := validateYAMLNode(&document, references, make(map[*yaml.Node]yamlVisitState)); err != nil {
		return yaml.Node{}, nil, err
	}
	return document, references, nil
}

func validateYAMLNode(node *yaml.Node, references map[Reference]struct{}, visits map[*yaml.Node]yamlVisitState) error {
	if node == nil {
		return nil
	}
	switch visits[node] {
	case yamlVisiting:
		return errors.New("YAML contains an alias cycle")
	case yamlVisited:
		return nil
	}
	visits[node] = yamlVisiting
	defer func() { visits[node] = yamlVisited }()
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := validateYAMLNode(child, references, visits); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return errors.New("YAML contains an invalid mapping")
		}
		keys := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return errors.New("YAML contains a non-scalar mapping key")
			}
			if _, duplicate := keys[key.Value]; duplicate {
				return fmt.Errorf("YAML contains duplicate key %q at line %d", key.Value, key.Line)
			}
			keys[key.Value] = struct{}{}
			if err := validateYAMLNode(node.Content[index+1], references, visits); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return nil
		}
		reference, escaped, matched, err := parseReference(node.Value)
		if err != nil {
			return fmt.Errorf("line %d: %w", node.Line, err)
		}
		if matched && !escaped {
			references[reference] = struct{}{}
		}
	case yaml.AliasNode:
		return validateYAMLNode(node.Alias, references, visits)
	}
	return nil
}

func sortedReferences(unique map[Reference]struct{}) []Reference {
	references := make([]Reference, 0, len(unique))
	for reference := range unique {
		references = append(references, reference)
	}
	slices.SortFunc(references, func(left, right Reference) int {
		if left.NamespaceKey != right.NamespaceKey {
			return bytes.Compare([]byte(left.NamespaceKey), []byte(right.NamespaceKey))
		}
		if left.ItemKey != right.ItemKey {
			return bytes.Compare([]byte(left.ItemKey), []byte(right.ItemKey))
		}
		return bytes.Compare([]byte(left.FieldKey), []byte(right.FieldKey))
	})
	return references
}
