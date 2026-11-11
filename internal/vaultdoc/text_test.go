package vaultdoc

import (
	"encoding/json"
	"os"
	"testing"
	"unicode"
)

// textVectors are shared with the Management UI (web/tests/vault-text.spec.js) so both sides apply one rule (#62).
type textVector struct {
	Input string `json:"input"`
	Want  string `json:"want"`
}

func loadTextVectors(t *testing.T) []textVector {
	t.Helper()
	encoded, err := os.ReadFile("testdata/text-normalization.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []textVector
	if err := json.Unmarshal(encoded, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 10 {
		t.Fatalf("only %d vectors", len(vectors))
	}
	return vectors
}

func TestNormalizeTextVectors(t *testing.T) {
	for _, vector := range loadTextVectors(t) {
		if got := NormalizeText(vector.Input); got != vector.Want {
			t.Errorf("NormalizeText(%q) = %q, want %q", vector.Input, got, vector.Want)
		}
		if got := NormalizeText(vector.Want); got != vector.Want {
			t.Errorf("NormalizeText is not idempotent for %q: %q", vector.Want, got)
		}
	}
}

func TestIsEdgeSpaceIsWhiteSpaceWithZeroWidthSpaceAndBOM(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		want := unicode.Is(unicode.White_Space, r) || r == 0x200B || r == 0xFEFF
		if got := IsEdgeSpace(r); got != want {
			t.Fatalf("IsEdgeSpace(U+%04X) = %v, want %v", r, got, want)
		}
	}
}

func TestNormalizeValueOnlyChangesTextAndNeverWritesThroughTheRequest(t *testing.T) {
	original := " http://service.example.com:8918 "
	request := Value{Text: &original}
	got := NormalizeValue(Text, request)
	if *got.Text != "http://service.example.com:8918" || original != " http://service.example.com:8918 " || got.Text == request.Text {
		t.Fatalf("Text normalization = %q, request now %q", *got.Text, original)
	}
	if secret := NormalizeValue(Secret, request); secret.Text != request.Text {
		t.Fatal("Secret value was normalized")
	}
	file := Value{File: &FileValue{Filename: "a", Bytes: []byte(" x ")}}
	if NormalizeValue(Text, file).File != file.File {
		t.Fatal("File value was replaced")
	}

	snapshot := Snapshot{
		Fields: []Field{{Key: "url", Name: "URL", Type: Text}, {Key: "password", Name: "Password", Type: Secret}},
		Variants: []Variant{{Values: map[string]Value{
			"url":      {Text: &original},
			"password": {Text: &original},
		}}},
	}
	NormalizeSnapshotText(snapshot)
	if *snapshot.Variants[0].Values["url"].Text != "http://service.example.com:8918" || *snapshot.Variants[0].Values["password"].Text != original || original != " http://service.example.com:8918 " {
		t.Fatal("snapshot normalization changed the wrong value")
	}
}
