package vaultdoc_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestEncodeValidatesAndCanonicalizesCompleteVaultSnapshot(t *testing.T) {
	username := "redis-user"
	password := "sentinel-secret"
	snapshot := vaultdoc.Snapshot{
		Fields: []vaultdoc.Field{
			{Key: "username", Name: "Username", Type: vaultdoc.Text},
			{Key: "password", Name: "Password", Type: vaultdoc.Secret},
			{Key: "tls_cert", Name: "TLS Certificate", Type: vaultdoc.File},
		},
		Variants: []vaultdoc.Variant{{
			ID:           "06060606060606060606060606060606",
			Environments: []string{"b", "a"},
			Values: map[string]vaultdoc.Value{
				"username": {Text: &username},
				"password": {Text: &password},
				"tls_cert": {File: &vaultdoc.FileValue{Filename: "server.pem", ContentType: "application/x-pem-file", Bytes: []byte("sentinel-file")}},
			},
		}},
	}
	encoded, err := vaultdoc.Encode(snapshot)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := vaultdoc.Decode(encoded.Plaintext)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	variant := decoded.Variants[snapshot.Variants[0].ID]
	if variant.Values["password"].Text == nil || *variant.Values["password"].Text != password {
		t.Fatalf("decoded password = %#v", variant.Values["password"])
	}
	if !bytes.Equal(variant.Values["tls_cert"].File.Bytes, []byte("sentinel-file")) {
		t.Fatalf("decoded File = %#v", variant.Values["tls_cert"].File)
	}

	reordered := snapshot
	reordered.Fields = []vaultdoc.Field{snapshot.Fields[2], snapshot.Fields[0], snapshot.Fields[1]}
	reordered.Variants[0].Environments = []string{"a", "b"}
	again, err := vaultdoc.Encode(reordered)
	if err != nil {
		t.Fatalf("Encode reordered Snapshot: %v", err)
	}
	if !bytes.Equal(again.Plaintext, encoded.Plaintext) || again.StructureSHA256 != encoded.StructureSHA256 {
		t.Fatal("semantic reordering changed canonical Vault Snapshot")
	}
}

func TestEncodeRejectsIncompleteOverlappingOrOversizedVaultSnapshots(t *testing.T) {
	text := "value"
	valid := func() vaultdoc.Snapshot {
		return vaultdoc.Snapshot{
			Fields: []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}},
			Variants: []vaultdoc.Variant{{
				ID:           "01010101010101010101010101010101",
				Environments: []string{"a"},
				Values:       map[string]vaultdoc.Value{"password": {Text: &text}},
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*vaultdoc.Snapshot)
	}{
		{"duplicate Field", func(snapshot *vaultdoc.Snapshot) { snapshot.Fields = append(snapshot.Fields, snapshot.Fields[0]) }},
		{"invalid Field type", func(snapshot *vaultdoc.Snapshot) { snapshot.Fields[0].Type = "password" }},
		{"missing Field value", func(snapshot *vaultdoc.Snapshot) { delete(snapshot.Variants[0].Values, "password") }},
		{"unknown Field value", func(snapshot *vaultdoc.Snapshot) { snapshot.Variants[0].Values["other"] = vaultdoc.Value{Text: &text} }},
		{"invalid Variant ID", func(snapshot *vaultdoc.Snapshot) { snapshot.Variants[0].ID = "variant-one" }},
		{"overlapping Environment", func(snapshot *vaultdoc.Snapshot) {
			snapshot.Variants = append(snapshot.Variants, vaultdoc.Variant{ID: "02020202020202020202020202020202", Environments: []string{"a"}, Values: map[string]vaultdoc.Value{"password": {Text: &text}}})
		}},
		{"text too large", func(snapshot *vaultdoc.Snapshot) {
			oversized := strings.Repeat("x", (512<<10)+1)
			snapshot.Variants[0].Values["password"] = vaultdoc.Value{Text: &oversized}
		}},
		{"File in Secret Field", func(snapshot *vaultdoc.Snapshot) {
			snapshot.Variants[0].Values["password"] = vaultdoc.Value{File: &vaultdoc.FileValue{Filename: "x", Bytes: []byte("x")}}
		}},
		{"File too large", func(snapshot *vaultdoc.Snapshot) {
			snapshot.Fields[0].Type = vaultdoc.File
			snapshot.Variants[0].Values["password"] = vaultdoc.Value{File: &vaultdoc.FileValue{Filename: "large.bin", Bytes: make([]byte, (5<<20)+1)}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := valid()
			test.mutate(&snapshot)
			if _, err := vaultdoc.Encode(snapshot); err == nil {
				t.Fatal("Encode accepted invalid Vault Snapshot")
			}
		})
	}
}

func TestDecodeRejectsUnknownOrTrailingPayloadData(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte(`{"variants":{},"secret":"leak"}`),
		[]byte(`{"variants":{}} {}`),
		bytes.Repeat([]byte("x"), (10<<20)+1),
	} {
		if _, err := vaultdoc.Decode(payload); err == nil {
			t.Fatal("Decode accepted malformed Vault payload")
		}
	}
}
