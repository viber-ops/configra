// Command go-licenses delivers reviewed material for an actual Configra binary.
// It does not infer legal permission from a classifier exit status.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed reviewed.json
var reviewBytes []byte

type profile struct {
	Target  string `json:"target"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	GOAMD64 string `json:"goamd64"`
	GOARM64 string `json:"goarm64"`
	Count   int    `json:"package_count"`
	Digest  string `json:"package_paths_sha256"`
}
type origin struct {
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	Hash   string `json:"hash"`
	SHA256 string `json:"sha256"`
}
type noticeFile struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	SHA256   string `json:"sha256"`
	Source   string `json:"source"`
	Verified bool   `json:"verified"`
	External bool   `json:"external_to_module"`
}
type sourceArchive struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Sum      string `json:"module_sum"`
	Verified bool   `json:"verified"`
}
type module struct {
	Path          string         `json:"module"`
	Version       string         `json:"version"`
	Sum           string         `json:"module_sum"`
	Licenses      []string       `json:"primary_license_identifiers"`
	Qualification string         `json:"qualification"`
	Origin        origin         `json:"origin"`
	Files         []noticeFile   `json:"files"`
	Source        *sourceArchive `json:"covered_source_archive"`
	TargetMask    int            `json:"target_mask"`
}
type review struct {
	Schema     int               `json:"schema_version"`
	Toolchain  string            `json:"toolchain"`
	Profiles   []profile         `json:"package_profiles"`
	TargetBits map[string]string `json:"target_bits"`
	Modules    []module          `json:"modules"`
}
type downloaded struct {
	Path, Version, Sum, Dir, Zip, Error string
	Origin                              *struct{ URL, Hash string }
}
type property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type deliveredFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

func main() {
	binary := flag.String("binary", "", "built Configra executable (never executed)")
	output := flag.String("out", "", "new directory for license materials")
	flag.Parse()
	if *binary == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: go-licenses -binary EXECUTABLE -out NEW_DIRECTORY (run in its source module)")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := collect(ctx, *binary, *output); err != nil {
		fmt.Fprintln(os.Stderr, "License material collection failed:", err)
		os.Exit(1)
	}
}

func collect(ctx context.Context, binary, output string) error {
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("output must not already exist: %s", output)
	}
	var approved review
	if err := json.Unmarshal(reviewBytes, &approved); err != nil {
		return err
	}
	if approved.Schema != 1 {
		return errors.New("unsupported review manifest")
	}
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("read executable build information: %w", err)
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if info.GoVersion != approved.Toolchain || settings["CGO_ENABLED"] != "0" || settings["-trimpath"] != "true" {
		return errors.New("toolchain, cgo or path-stripping profile has not been reviewed")
	}
	for _, key := range []string{"-tags", "-race", "GOEXPERIMENT"} {
		if settings[key] != "" && settings[key] != "false" {
			return fmt.Errorf("unreviewed build setting %s", key)
		}
	}
	command := path.Base(info.Path)
	if (command != "configra" && command != "configra-kubernetes") || info.Path != info.Main.Path+"/cmd/"+command {
		return errors.New("binary is not one of the reviewed Configra commands")
	}
	target := command + "/" + settings["GOOS"] + "/" + settings["GOARCH"]
	var selected *profile
	for i := range approved.Profiles {
		if approved.Profiles[i].Target == target {
			selected = &approved.Profiles[i]
		}
	}
	if selected == nil || settings["GOAMD64"] != selected.GOAMD64 || settings["GOARM64"] != selected.GOARM64 {
		return fmt.Errorf("target/CPU profile requires review: %s", target)
	}
	targetBit := 0
	for i := 0; i < 8; i++ {
		if approved.TargetBits[fmt.Sprint(i)] == target {
			targetBit = 1 << i
		}
	}
	if targetBit == 0 {
		return errors.New("reviewed target mask is missing")
	}
	env := commandEnv(map[string]string{"GOTOOLCHAIN": approved.Toolchain, "GOWORK": "off", "GOFLAGS": "-mod=readonly",
		"CGO_ENABLED": "0", "GOOS": selected.GOOS, "GOARCH": selected.GOARCH,
		"GOAMD64": selected.GOAMD64, "GOARM64": selected.GOARM64, "GOEXPERIMENT": ""})
	// The package guard includes stdlib and source packages, not just module names.
	listing, err := run(ctx, env, "list", "-deps", "-f", "{{.ImportPath}}{{with .Module}}\t{{.Path}}\t{{.Version}}{{if .Replace}}\tREPLACED{{end}}{{end}}", info.Path)
	if err != nil {
		return err
	}
	var packages []string
	sourceModules := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(listing), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 1 && len(fields) != 3 {
			return errors.New("source dependency replacement or malformed package metadata")
		}
		packages = append(packages, fields[0])
		if len(fields) == 3 && fields[1] != info.Main.Path {
			sourceModules[fields[1]] = fields[2]
		}
	}
	sort.Strings(packages)
	if len(packages) != selected.Count || digest([]byte(strings.Join(packages, "\n")+"\n")) != selected.Digest {
		return fmt.Errorf("package selection changed for %s; review new imports before distributing", target)
	}
	if len(sourceModules) != len(info.Deps) {
		return errors.New("source and binary dependency sets differ")
	}
	for _, dependency := range info.Deps {
		if dependency.Replace != nil || sourceModules[dependency.Path] != dependency.Version {
			return fmt.Errorf("source and binary disagree about %s", dependency.Path)
		}
	}
	if _, err := run(ctx, env, "mod", "verify"); err != nil {
		return err
	}
	goRootBytes, err := run(ctx, env, "env", "GOROOT")
	if err != nil {
		return err
	}
	goRoot := strings.TrimSpace(string(goRootBytes))
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(output), ".configra-module-notices-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging) // Only this invocation's newly created staging directory.
	var components []map[string]any
	var refs []string
	var notices strings.Builder
	var access strings.Builder
	fmt.Fprintln(&notices, "Configra Go dependency license materials\n\nLicense identifiers are source-text evidence. Read the per-module qualifications for file-specific terms and retained supersets.")
	fmt.Fprintln(&access, "# Covered source access\n\nOriginal Configra code remains Apache-2.0. Covered third-party source retains its original terms. No covered dependency files were modified for this build.")
	for _, dependency := range info.Deps {
		var m *module
		for i := range approved.Modules {
			candidate := &approved.Modules[i]
			if candidate.Path == dependency.Path && candidate.Version == dependency.Version {
				m = candidate
				break
			}
		}
		if m == nil || m.Sum != dependency.Sum || m.TargetMask&targetBit == 0 || len(m.Files) == 0 || len(m.Licenses) == 0 {
			return fmt.Errorf("module/version/checksum requires license review: %s@%s", dependency.Path, dependency.Version)
		}
		metadata, err := run(ctx, env, "mod", "download", "-json", m.Path+"@"+m.Version)
		if err != nil {
			return err
		}
		var download downloaded
		if err := json.Unmarshal(metadata, &download); err != nil {
			return err
		}
		if download.Error != "" || download.Path != m.Path || download.Version != m.Version || download.Sum != m.Sum || download.Dir == "" {
			return fmt.Errorf("downloaded source does not match reviewed %s@%s", m.Path, m.Version)
		}
		if m.Origin.Kind == "vcs" && download.Origin != nil && download.Origin.Hash != m.Origin.Hash {
			return fmt.Errorf("source origin changed for %s", m.Path)
		}
		if m.Origin.Kind == "module_archive" {
			hash, err := fileDigest(download.Zip)
			if err != nil {
				return err
			}
			if hash != m.Origin.SHA256 {
				return fmt.Errorf("source archive changed for %s", m.Path)
			}
		}
		prefix := "files/" + m.Path + "@" + m.Version
		if !safePath(prefix) {
			return errors.New("invalid reviewed module path")
		}
		var delivered []deliveredFile
		fmt.Fprintf(&notices, "\n## %s@%s\n\n%s\nModule checksum: %s\nSource: %s %s\n", m.Path, m.Version, m.Qualification, m.Sum, m.Origin.URL, m.Origin.Hash)
		for _, file := range m.Files {
			if !file.Verified || !safePath(file.Path) {
				return errors.New("unverified or invalid notice path")
			}
			from := filepath.Join(download.Dir, filepath.FromSlash(file.Path))
			if file.External {
				if file.Source != "https://github.com/golang/go/blob/go1.26.7/LICENSE" {
					return errors.New("unreviewed external notice source")
				}
				from = filepath.Join(goRoot, "LICENSE")
			}
			content, err := boundedFile(from, 2<<20)
			if err != nil {
				return err
			}
			if digest(content) != file.SHA256 {
				return fmt.Errorf("notice bytes changed: %s/%s", m.Path, file.Path)
			}
			to := prefix + "/" + file.Path
			switch path.Ext(to) {
			case ".go", ".s", ".h":
				to += ".txt"
			}
			if err := write(staging, to, content); err != nil {
				return err
			}
			delivered = append(delivered, deliveredFile{Path: to, SHA256: file.SHA256, Source: file.Source})
			fmt.Fprintf(&notices, "\n### %s\nSource: %s\nSHA-256: %s\n\n%s\n", file.Path, file.Source, file.SHA256, content)
		}
		if m.Source != nil {
			if !m.Source.Verified || m.Source.Sum != m.Sum || m.Path != "github.com/go-sql-driver/mysql" {
				return errors.New("covered source requires review")
			}
			content, err := boundedFile(download.Zip, 16<<20)
			if err != nil {
				return err
			}
			if digest(content) != m.Source.SHA256 {
				return errors.New("covered source archive checksum mismatch")
			}
			file := "sources/mysql-" + m.Version + ".zip"
			if err := write(staging, file, content); err != nil {
				return err
			}
			fmt.Fprintf(&access, "\n## %s %s (MPL-2.0)\n\nExact unmodified module source: [%s](%s).\nUpstream: %s\nSHA-256: `%s`.\nThe original LICENSE and AUTHORS remain inside that archive and the notice collection.\n", m.Path, m.Version, file, file, m.Source.URL, m.Source.SHA256)
		}
		purl := "pkg:golang/" + m.Path + "@" + m.Version
		refs = append(refs, purl)
		var licenseEvidence []map[string]any
		for _, id := range m.Licenses {
			licenseEvidence = append(licenseEvidence, map[string]any{"license": map[string]string{"id": id}})
		}
		filesJSON, _ := json.Marshal(delivered)
		components = append(components, map[string]any{
			"type": "library", "bom-ref": purl, "name": m.Path, "version": m.Version, "purl": purl,
			"evidence":           map[string]any{"licenses": licenseEvidence},
			"properties":         []property{{"configra:go:module-sum", m.Sum}, {"configra:license-scope", m.Qualification}, {"configra:license-files", string(filesJSON)}},
			"externalReferences": []map[string]string{{"type": "distribution", "url": moduleURL(m.Path, m.Version)}},
		})
	}
	binaryHash, err := fileDigest(binary)
	if err != nil {
		return err
	}
	rootRef := "urn:sha256:" + binaryHash
	bom := map[string]any{
		"$schema": "http://cyclonedx.org/schema/bom-1.6.schema.json", "bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
		"metadata": map[string]any{
			"component": map[string]any{"type": "application", "bom-ref": rootRef, "name": command,
				"hashes":      []map[string]string{{"alg": "SHA-256", "content": binaryHash}},
				"description": "Linked Go module source/license evidence only. Go runtime notices, embedded UI and container CA material are supplied separately."},
			"properties": []property{{"configra:target", target}, {"configra:go:version", info.GoVersion}, {"configra:package-selection-sha256", selected.Digest}},
		},
		"components": components, "dependencies": []map[string]any{{"ref": rootRef, "dependsOn": refs}},
	}
	encoded, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return err
	}
	for name, content := range map[string][]byte{"sbom.cdx.json": append(encoded, '\n'), "THIRD_PARTY_NOTICES.txt": []byte(notices.String()), "SOURCE_ACCESS.md": []byte(access.String())} {
		if err := write(staging, name, content); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return errors.New("output appeared during collection; refusing to replace it")
	}
	if err := os.Rename(staging, output); err != nil {
		return err
	}
	fmt.Printf("Verified %d linked module versions and their notice materials for %s.\n", len(components), target)
	return nil
}

func commandEnv(overrides map[string]string) []string {
	var result []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := overrides[key]; !ok {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}
func run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "go", args...)
	command.Env = env
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}
func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
func moduleURL(module, version string) string {
	escape := func(value string) string {
		var result strings.Builder
		for _, char := range value {
			if char >= 'A' && char <= 'Z' {
				result.WriteByte('!')
				result.WriteRune(char + ('a' - 'A'))
			} else {
				result.WriteRune(char)
			}
		}
		return result.String()
	}
	return "https://proxy.golang.org/" + escape(module) + "/@v/" + escape(version) + ".zip"
}
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func boundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("notice/source file exceeds collection limit")
	}
	return content, nil
}
func safePath(value string) bool {
	return value != "." && value != "" && !strings.Contains(value, "\\") && !path.IsAbs(value) && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}
func write(directory, name string, content []byte) error {
	if !safePath(name) {
		return errors.New("invalid material output path")
	}
	full := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o644)
}
