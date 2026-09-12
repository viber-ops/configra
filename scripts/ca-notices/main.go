// Command ca-notices preserves the reviewed system trust-bundle materials.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const sourceURL = "https://security.debian.org/debian-security/pool/updates/main/c/ca-certificates/ca-certificates_20250419~deb12u1.tar.xz"
const sourceHash = "b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d"
const bundleHash = "714d457d580922dbf1d0be8bd35ba236a842b50b0072ae791582a19adef772a5"
const bundlePath = "etc/ssl/certs/ca-certificates.crt"

type material struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

var inputs = []struct{ source, output, hash string }{
	{"usr/share/doc/ca-certificates/copyright", "copyright.txt", "e85e1bcad3a915dc7e6f41412bc5bdeba275cadd817896ea0451f2140a93967c"},
	{"usr/share/common-licenses/MPL-2.0", "MPL-2.0.txt", "fab3dd6bdab226f1c08630b1dd917e11fcb4ec5e1e020e2c16f83a0a13863e85"},
	{"usr/share/common-licenses/GPL-2", "GPL-2.0.txt", "8177f97513213526df2cf6184d8ff986c675afb514d4e68a404010521b880643"},
	{"etc/ca-certificates.conf", "ca-certificates.conf", "ab7339d40969fb1084cb23011fbdae7cc8edb46ac7e410897bb6b7016f07ed7f"},
}

func main() {
	root := flag.String("rootfs", "/", "filesystem supplying the system CA bundle")
	source := flag.String("source", "", "exact Debian source archive")
	output := flag.String("out", "", "new material output directory")
	flag.Parse()
	if *source == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: ca-notices -rootfs ROOT -source SOURCE.tar.xz -out NEW_DIRECTORY")
		os.Exit(2)
	}
	if err := collect(*root, *source, *output); err != nil {
		fmt.Fprintln(os.Stderr, "CA material collection failed:", err)
		os.Exit(1)
	}
}

func collect(root, source, output string) error {
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return errors.New("output must not already exist")
	}
	archive, err := verifiedFile(source, sourceHash, 8<<20)
	if err != nil {
		return fmt.Errorf("source archive: %w", err)
	}
	if _, err := verifiedFile(filepath.Join(root, bundlePath), bundleHash, 2<<20); err != nil {
		return fmt.Errorf("system trust bundle requires review: %w", err)
	}
	contents := map[string][]byte{"source.tar.xz": archive}
	files := []material{{Path: "source.tar.xz", SHA256: sourceHash}}
	for _, input := range inputs {
		content, err := verifiedFile(filepath.Join(root, input.source), input.hash, 2<<20)
		if err != nil {
			return fmt.Errorf("%s requires review: %w", input.source, err)
		}
		contents[input.output] = content
		files = append(files, material{Path: input.output, SHA256: input.hash})
	}
	access := []byte("# System CA trust bundle source and licenses\n\n" +
		"The runtime bundle is copied without modification from the reviewed Debian ca-certificates 20250419~deb12u1 package. Its SHA-256 is " + bundleHash + ".\n\n" +
		"The complete original source is included in source.tar.xz (SHA-256 " + sourceHash + "). Upstream: " + sourceURL + "\n\n" +
		"Inside that archive, ca-certificates/mozilla/certdata.txt and ca-certificates/mozilla/nssckbi.h carry Mozilla Contributors attribution and MPL-2.0 terms. See copyright.txt and MPL-2.0.txt.\n\n" +
		"The archive also preserves Debian conversion/update/packaging scripts under GPL-2.0-or-later; GPL-2.0.txt accompanies those sources. These scripts are not installed as runtime programs in the Configra scratch image. This does not relicense Configra's separate original code away from Apache-2.0.\n\n" +
		"ca-certificates.conf records the reviewed selection configuration. A different bundle, selection, source archive or legal text requires a new review; this is not a collector for user-managed CAs or private keys.\n")
	contents["SOURCE_ACCESS.md"] = access
	files = append(files, material{Path: "SOURCE_ACCESS.md", SHA256: digest(access)})
	metadata := map[string]any{
		"formatVersion": 1, "package": "ca-certificates", "version": "20250419~deb12u1",
		"bundle":      map[string]string{"path": "/" + bundlePath, "sha256": bundleHash},
		"source":      map[string]string{"path": "source.tar.xz", "url": sourceURL, "sha256": sourceHash},
		"dataLicense": "MPL-2.0", "sourceScriptsLicense": "GPL-2.0-or-later", "files": files,
	}
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	contents["metadata.json"] = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(output), ".configra-ca-notices-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for name, content := range contents {
		if err := os.WriteFile(filepath.Join(staging, name), content, 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return errors.New("output appeared during collection; refusing to replace it")
	}
	if err := os.Rename(staging, output); err != nil {
		return err
	}
	fmt.Println("Verified and preserved ca-certificates 20250419~deb12u1 source, licenses and runtime-bundle identity.")
	return nil
}

func digest(content []byte) string { sum := sha256.Sum256(content); return hex.EncodeToString(sum[:]) }
func verifiedFile(path, expected string, limit int64) ([]byte, error) {
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
		return nil, errors.New("input exceeds collection limit")
	}
	if digest(content) != expected {
		return nil, errors.New("input does not match its reviewed SHA-256")
	}
	return content, nil
}
