# Go 1.26.10 runtime and standard-library notices

This directory retains original license texts, patent grants and source-embedded
notices from the Go 1.26.10 toolchain used to build Configra. Source files are kept
whole so their copyright and permission blocks are not shortened or rewritten.
Source filenames ending in `.go`, `.s`, or `.h` have an additional `.txt` suffix
to keep Go tooling from treating the notice collection as buildable packages.
File contents are unchanged.
The Apache-2.0 text referenced by `src/internal/profile/graph.go` is also supplied
with Configra's first-party license materials.

This is a conservative union for Linux/macOS, amd64/arm64, with cgo disabled and
without the `boringcrypto` experiment. It includes attribution context for Go's
vendored packages, fiat-crypto, Lucent/Vita Nuova memmove, Sun/Cephes math code,
and other source-embedded notices. Inclusion does not assert that every named
implementation survives linking or that BoringSSL is used by these binaries.
Re-review this collection when the toolchain, targets, cgo or experiments change.

Upstream source: https://go.dev/dl/go1.26.10.src.tar.gz

Archive SHA-256:
`fd63e883ba2f2bfca93c1646b31ebef63e2b1e4d2b317c9789d976a68d81f36f`

Tree: https://github.com/golang/go/tree/go1.26.10

Paths retain their location relative to the archive's `go/` root. This collection
does not replace license materials for Configra's Go modules, bundled web UI,
or a container's CA bundle; those components have independent terms.
