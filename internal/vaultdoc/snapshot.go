package vaultdoc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

const (
	MaxTextBytes     = 512 << 10
	MaxFileBytes     = 5 << 20
	MaxSnapshotBytes = 10 << 20
)

type FieldType string

const (
	Text   FieldType = "text"
	Secret FieldType = "secret"
	File   FieldType = "file"
)

type Field struct {
	Key  string    `json:"key"`
	Name string    `json:"name"`
	Type FieldType `json:"type"`
}

type Variant struct {
	ID           string           `json:"id"`
	Environments []string         `json:"environments"`
	Values       map[string]Value `json:"values,omitempty"`
}

type Snapshot struct {
	Fields   []Field   `json:"fields"`
	Variants []Variant `json:"variants"`
}

type Value struct {
	Text *string    `json:"text,omitempty"`
	File *FileValue `json:"file,omitempty"`
}

type FileValue struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       []byte `json:"bytes"`
}

type Payload struct {
	Variants map[string]VariantValues `json:"variants"`
}

type VariantValues struct {
	Values map[string]Value `json:"values"`
}

type Encoded struct {
	Plaintext       []byte
	StructureSHA256 [sha256.Size]byte
}

func Encode(snapshot Snapshot) (Encoded, error) {
	if len(snapshot.Fields) == 0 || len(snapshot.Variants) == 0 {
		return Encoded{}, errors.New("Vault Snapshot requires at least one Field and Variant")
	}
	fields := slices.Clone(snapshot.Fields)
	slices.SortFunc(fields, func(left, right Field) int { return bytes.Compare([]byte(left.Key), []byte(right.Key)) })
	fieldTypes := make(map[string]FieldType, len(fields))
	for _, field := range fields {
		if !validResourceKey(field.Key) || field.Name == "" || len(field.Name) > 255 {
			return Encoded{}, fmt.Errorf("invalid Vault Field %q", field.Key)
		}
		if field.Type != Text && field.Type != Secret && field.Type != File {
			return Encoded{}, fmt.Errorf("invalid type for Vault Field %q", field.Key)
		}
		if _, duplicate := fieldTypes[field.Key]; duplicate {
			return Encoded{}, fmt.Errorf("duplicate Vault Field %q", field.Key)
		}
		fieldTypes[field.Key] = field.Type
	}

	type structureVariant struct {
		ID           string   `json:"id"`
		Environments []string `json:"environments"`
	}
	structureVariants := make([]structureVariant, 0, len(snapshot.Variants))
	payload := Payload{Variants: make(map[string]VariantValues, len(snapshot.Variants))}
	boundEnvironments := make(map[string]string)
	for _, variant := range snapshot.Variants {
		if !validVariantID(variant.ID) {
			return Encoded{}, fmt.Errorf("invalid Vault Variant ID %q", variant.ID)
		}
		if _, duplicate := payload.Variants[variant.ID]; duplicate {
			return Encoded{}, fmt.Errorf("duplicate Vault Variant ID %q", variant.ID)
		}
		environments := slices.Clone(variant.Environments)
		slices.Sort(environments)
		for index, environment := range environments {
			if !validResourceKey(environment) {
				return Encoded{}, fmt.Errorf("invalid Environment Key %q", environment)
			}
			if index > 0 && environment == environments[index-1] {
				return Encoded{}, fmt.Errorf("Environment %q is repeated in Variant %s", environment, variant.ID)
			}
			if previous, duplicate := boundEnvironments[environment]; duplicate {
				return Encoded{}, fmt.Errorf("Environment %q belongs to Variants %s and %s", environment, previous, variant.ID)
			}
			boundEnvironments[environment] = variant.ID
		}
		if len(variant.Values) != len(fieldTypes) {
			return Encoded{}, fmt.Errorf("Variant %s does not contain exactly one value for every Field", variant.ID)
		}
		for key := range variant.Values {
			if _, ok := fieldTypes[key]; !ok {
				return Encoded{}, fmt.Errorf("Variant %s contains unknown Field %q", variant.ID, key)
			}
		}
		for key, fieldType := range fieldTypes {
			value, ok := variant.Values[key]
			if !ok {
				return Encoded{}, fmt.Errorf("Variant %s is missing Field %q", variant.ID, key)
			}
			if err := validateValue(fieldType, value); err != nil {
				return Encoded{}, fmt.Errorf("Variant %s Field %q: %w", variant.ID, key, err)
			}
		}
		payload.Variants[variant.ID] = VariantValues{Values: variant.Values}
		structureVariants = append(structureVariants, structureVariant{ID: variant.ID, Environments: environments})
	}
	slices.SortFunc(structureVariants, func(left, right structureVariant) int {
		return bytes.Compare([]byte(left.ID), []byte(right.ID))
	})
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return Encoded{}, fmt.Errorf("encode Vault Snapshot: %w", err)
	}
	if len(plaintext) > MaxSnapshotBytes {
		clear(plaintext)
		return Encoded{}, errors.New("Vault Snapshot exceeds 10 MiB")
	}
	structure, err := json.Marshal(struct {
		Fields   []Field            `json:"fields"`
		Variants []structureVariant `json:"variants"`
	}{Fields: fields, Variants: structureVariants})
	if err != nil {
		clear(plaintext)
		return Encoded{}, fmt.Errorf("encode Vault structure: %w", err)
	}
	digest := sha256.Sum256(structure)
	clear(structure)
	return Encoded{Plaintext: plaintext, StructureSHA256: digest}, nil
}

func Decode(plaintext []byte) (Payload, error) {
	if len(plaintext) > MaxSnapshotBytes {
		return Payload{}, errors.New("Vault Snapshot exceeds 10 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	var payload Payload
	if err := decoder.Decode(&payload); err != nil {
		return Payload{}, fmt.Errorf("decode Vault Snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Payload{}, errors.New("Vault Snapshot contains trailing data")
	}
	if len(payload.Variants) == 0 {
		return Payload{}, errors.New("Vault Snapshot contains no Variants")
	}
	for id, variant := range payload.Variants {
		if !validVariantID(id) || len(variant.Values) == 0 {
			return Payload{}, errors.New("Vault Snapshot contains invalid Variant metadata")
		}
		for key, value := range variant.Values {
			if !validResourceKey(key) || (value.Text == nil) == (value.File == nil) {
				return Payload{}, errors.New("Vault Snapshot contains invalid Field value")
			}
			if value.Text != nil && len(*value.Text) > MaxTextBytes {
				return Payload{}, errors.New("Vault Snapshot Text exceeds 512 KiB")
			}
			if value.File != nil && (len(value.File.Bytes) > MaxFileBytes || !validFileMetadata(value.File)) {
				return Payload{}, errors.New("Vault Snapshot File is invalid")
			}
		}
	}
	return payload, nil
}

func validateValue(fieldType FieldType, value Value) error {
	switch fieldType {
	case Text, Secret:
		if value.Text == nil || value.File != nil {
			return errors.New("Text/Secret Field requires exactly one text value")
		}
		if len(*value.Text) > MaxTextBytes {
			return errors.New("Text/Secret Field exceeds 512 KiB")
		}
	case File:
		if value.Text != nil || value.File == nil {
			return errors.New("File Field requires exactly one File value")
		}
		if len(value.File.Bytes) > MaxFileBytes {
			return errors.New("File Field exceeds 5 MiB")
		}
		if !validFileMetadata(value.File) {
			return errors.New("File metadata is invalid")
		}
	}
	return nil
}

func validFileMetadata(file *FileValue) bool {
	return file.Filename != "" && len(file.Filename) <= 255 && len(file.ContentType) <= 255
}

func validVariantID(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
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
