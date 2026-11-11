package vaultdoc

import "strings"

// NormalizeText returns a Text Field value as Configra stores it (#62). A single-line value loses its leading and
// trailing whitespace, so a pasted " http://host:8918\n" is stored as "http://host:8918"; spaces inside the value are
// kept. A value that still contains a line feed or carriage return once its edges are removed is multi-line text,
// such as a certificate, a key or a script, and is returned exactly as given, because a final newline or a leading
// indent can matter there. Secret and File values are never normalized.
//
// The set of edge characters is listed explicitly so the server, the Management UI and their tests agree: Go's
// unicode.IsSpace and JavaScript's String.prototype.trim differ on U+0085 and U+FEFF, and neither removes U+200B.
func NormalizeText(value string) string {
	trimmed := strings.TrimFunc(value, IsEdgeSpace)
	if len(trimmed) == len(value) || strings.ContainsAny(trimmed, "\r\n") {
		return value
	}
	return trimmed
}

// IsEdgeSpace reports whether NormalizeText removes r from the edges of a single-line Text value: the Unicode
// White_Space characters (tab, line feed, vertical tab, form feed, carriage return, space, U+0085, U+00A0, U+1680,
// U+2000–U+200A, U+2028, U+2029, U+202F, U+205F and the full-width space U+3000), plus the invisible U+200B zero width
// space and U+FEFF byte order mark that copying from documents and files leaves behind.
func IsEdgeSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x0085, 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0x200B, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// NormalizeValue returns value with its text normalized when fieldType is Text. It never writes through value.Text,
// so a caller's request, and the idempotency digest computed from it, keep the bytes the client sent.
func NormalizeValue(fieldType FieldType, value Value) Value {
	if fieldType != Text || value.Text == nil {
		return value
	}
	text := NormalizeText(*value.Text)
	value.Text = &text
	return value
}

// NormalizeSnapshotText normalizes every Text Field value in snapshot, in place. Callers pass a snapshot they own,
// such as a clone of a request. Values of Fields the snapshot does not define are left for Encode to reject.
func NormalizeSnapshotText(snapshot Snapshot) {
	types := make(map[string]FieldType, len(snapshot.Fields))
	for _, field := range snapshot.Fields {
		types[field.Key] = field.Type
	}
	for _, variant := range snapshot.Variants {
		for key, value := range variant.Values {
			variant.Values[key] = NormalizeValue(types[key], value)
		}
	}
}
