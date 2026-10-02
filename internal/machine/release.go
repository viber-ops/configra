package machine

import (
	"path"
	"strings"
)

// ReleaseSpec names exact existing revisions. Paths are relative bundle paths,
// never commands, absolute paths, or paths to read on the server.
type ReleaseSpec struct {
	ConfigRevision uint64           `json:"config_revision"`
	ConfigPath     string           `json:"config_path"`
	Files          []ReleaseFileRef `json:"files"`
}
type ReleaseFileRef struct {
	Namespace string `json:"namespace"`
	Item      string `json:"item"`
	Field     string `json:"field"`
	Path      string `json:"path"`
	Revision  uint64 `json:"revision"`
}
type ReleaseManifest struct {
	Environment string `json:"environment"`
	ConfigKey   string `json:"config_key"`
	ReleaseKey  string `json:"release_key"`
	ReleaseSpec
	VaultRevisions map[string]uint64 `json:"vault_revisions"`
}
type ReleaseState struct {
	ReleaseKey string `json:"release_key"`
	Generation uint64 `json:"generation"`
}
type ReleaseFile struct {
	Path  string `json:"path"`
	Bytes []byte `json:"bytes"`
}
type ReleaseBundle struct {
	Manifest   ReleaseManifest `json:"manifest"`
	Generation uint64          `json:"generation"`
	Digest     string          `json:"digest"`
	Config     ResolvedConfig  `json:"config"`
	Files      []ReleaseFile   `json:"files"`
	ETag       string          `json:"-"`
}

func ValidReleasePath(value string) bool {
	if value == "" || len(value) > 255 || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.ContainsAny(value, "\\:\x00\r\n") {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
