package binding

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

func EnvironmentValues(content []byte) (map[string][]byte, error) {
	if len(content) > 128<<10 {
		return nil, errors.New("environment mapping exceeds 128 KiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if decoder.Decode(&document) != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("flat mapping required")
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, errors.New("one document required")
	}
	result := map[string][]byte{}
	mapping := document.Content[0]
	if len(mapping.Content)%2 != 0 {
		return nil, errors.New("invalid mapping")
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		key, value := mapping.Content[index], mapping.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !environmentName.MatchString(key.Value) {
			return nil, errors.New("invalid environment name")
		}
		if _, duplicate := result[key.Value]; duplicate {
			return nil, errors.New("duplicate environment name")
		}
		visited := map[*yaml.Node]bool{}
		for value.Kind == yaml.AliasNode {
			if visited[value] || value.Alias == nil {
				return nil, errors.New("invalid alias")
			}
			visited[value] = true
			value = value.Alias
		}
		if value.Kind != yaml.ScalarNode || strings.ContainsRune(value.Value, '\x00') {
			return nil, errors.New("scalar environment value required")
		}
		switch value.Tag {
		case "!!str", "!!int", "!!float", "!!bool", "!!timestamp":
		default:
			return nil, errors.New("unsupported environment value")
		}
		result[key.Value] = []byte(value.Value)
	}
	return result, nil
}
