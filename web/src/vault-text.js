// The Vault Text rule from the server (internal/vaultdoc/text.go, #62), applied in the editor so people see what a
// save stores. A single-line Text value loses its leading and trailing whitespace; a value that still contains a line
// feed or carriage return once its edges are removed is multi-line text and keeps every character. Secret and File
// values are never changed. The edge set is explicit because String.prototype.trim differs from the server on U+0085
// and U+FEFF and misses U+200B; every character in it is a single UTF-16 code unit.
export function isVaultEdgeSpace(code) {
  return (code >= 0x09 && code <= 0x0d) || code === 0x20 || code === 0x85 || code === 0xa0 || code === 0x1680 ||
    (code >= 0x2000 && code <= 0x200b) || code === 0x2028 || code === 0x2029 || code === 0x202f || code === 0x205f ||
    code === 0x3000 || code === 0xfeff;
}

export function normalizeVaultText(value) {
  let start = 0;
  let end = value.length;
  while (start < end && isVaultEdgeSpace(value.charCodeAt(start))) start += 1;
  while (end > start && isVaultEdgeSpace(value.charCodeAt(end - 1))) end -= 1;
  if (start === 0 && end === value.length) return value;
  const trimmed = value.slice(start, end);
  return trimmed.includes('\n') || trimmed.includes('\r') ? value : trimmed;
}

export function hasVaultEdgeSpace(value) {
  return value.length > 0 && (isVaultEdgeSpace(value.charCodeAt(0)) || isVaultEdgeSpace(value.charCodeAt(value.length - 1)));
}
