package configdoc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
)

func Merge(format Format, source, target []byte) (Document, error) {
	canonicalSource, err := Canonicalize(format, source)
	if err != nil {
		return Document{}, fmt.Errorf("validate Merge Source: %w", err)
	}
	canonicalTarget, err := Canonicalize(format, target)
	if err != nil {
		return Document{}, fmt.Errorf("validate Merge Target: %w", err)
	}
	switch format {
	case JSON:
		sourceValue, err := decodeCanonicalJSON(canonicalSource.Content)
		if err != nil {
			return Document{}, err
		}
		targetValue, err := decodeCanonicalJSON(canonicalTarget.Content)
		if err != nil {
			return Document{}, err
		}
		merged := mergeJSONValue(sourceValue, targetValue)
		content, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return Document{}, fmt.Errorf("format merged JSON: %w", err)
		}
		return Canonicalize(JSON, append(content, '\n'))
	case YAML:
		sourceDocument, _, err := parseYAML(canonicalSource.Content)
		if err != nil {
			return Document{}, err
		}
		targetDocument, _, err := parseYAML(canonicalTarget.Content)
		if err != nil {
			return Document{}, err
		}
		targetDocument.Content[0] = mergeYAMLNode(sourceDocument.Content[0], targetDocument.Content[0])
		var output bytes.Buffer
		encoder := yaml.NewEncoder(&output)
		encoder.SetIndent(2)
		if err := encoder.Encode(&targetDocument); err != nil {
			return Document{}, fmt.Errorf("format merged YAML: %w", err)
		}
		if err := encoder.Close(); err != nil {
			return Document{}, fmt.Errorf("close merged YAML formatter: %w", err)
		}
		return Canonicalize(YAML, output.Bytes())
	default:
		return Document{}, fmt.Errorf("unsupported Config format %q", format)
	}
}

func decodeCanonicalJSON(content []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder, 0)
	if err != nil {
		return nil, fmt.Errorf("decode canonical JSON: %w", err)
	}
	return value, nil
}

func mergeJSONValue(source, target any) any {
	sourceMapping, sourceIsMapping := source.(map[string]any)
	targetMapping, targetIsMapping := target.(map[string]any)
	if !sourceIsMapping || !targetIsMapping {
		return source
	}
	for key, sourceValue := range sourceMapping {
		if targetValue, exists := targetMapping[key]; exists {
			targetMapping[key] = mergeJSONValue(sourceValue, targetValue)
		} else {
			targetMapping[key] = sourceValue
		}
	}
	return targetMapping
}

func mergeYAMLNode(source, target *yaml.Node) *yaml.Node {
	if source.Kind != yaml.MappingNode || target.Kind != yaml.MappingNode {
		return source
	}
	targetIndexes := make(map[string]int, len(target.Content)/2)
	for index := 0; index < len(target.Content); index += 2 {
		targetIndexes[target.Content[index].Value] = index
	}
	for index := 0; index < len(source.Content); index += 2 {
		key := source.Content[index]
		value := source.Content[index+1]
		if targetIndex, exists := targetIndexes[key.Value]; exists {
			target.Content[targetIndex+1] = mergeYAMLNode(value, target.Content[targetIndex+1])
		} else {
			target.Content = append(target.Content, key, value)
			targetIndexes[key.Value] = len(target.Content) - 2
		}
	}
	return target
}
