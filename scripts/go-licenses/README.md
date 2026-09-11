# Go dependency material collector

Build once for the host, then run from the matching source module. The inspected
executable can target any of the four supported release platforms; it is not run.

```sh
# From the Configra checkout:
GOTOOLCHAIN=go1.26.7 GOWORK=off GOFLAGS=-mod=readonly \
  go build -o .cache/release-tools/configra-go-licenses ./scripts/go-licenses
.cache/release-tools/configra-go-licenses \
  -binary /absolute/path/to/configra \
  -out /absolute/path/to/new/module-materials
```

For the Kubernetes executable, run the collector from `kubernetes/`. Normal
release and Docker builds invoke it automatically. The output directory must be
new. Failed collection does not publish the temporary material directory.

The collector compares the binary's target, toolchain and module records with
target-matched source loading, verifies the package-selection digest (including
stdlib), runs `go mod verify`, and checks each reviewed notice's original bytes.
The MySQL driver's exact MPL-covered source ZIP is included with source-access
instructions. Notice-bearing source/assembly files receive `.txt` suffixes so
they do not become buildable packages under `dist/`.

`sbom.cdx.json` is a **Go application-module inventory**, bound to the binary's
SHA-256. License identifiers are recorded as evidence, with per-module scope
qualifications. It does not replace the separately delivered Go runtime, UI, or
container CA materials, and is not a claim of complete image inventory.

## Updating the reviewed inputs

Use [the transitive review](../../docs/research/transitive-go-license-inventory.md)
as the source record. Do not regenerate `reviewed.json` merely to make a failed
build pass. Review new imports even when module versions have not changed: a new
package can bring a nested license or source-embedded attribution.

1. Build the new candidate with the intended target flags and inspect its module
   records. Check the canonical source archives/module sums and available VCS
   origins; record an archive identity when a full commit is genuinely unknown.
2. Review root and nested licenses, NOTICE/AUTHORS/PATENTS, and source/assembly
   exceptions in the selected packages. Preserve mixed/file-specific terms.
3. Record notice hashes, module sums, target masks, and package-selection guards
   in the review and manifest only after that inspection. Hash the bytewise-sorted
   `go list -deps -f '{{.ImportPath}}'` paths with LF separators and a final LF.
4. Run the collector process regression, all four release builds, and actual-image
   verification. Validate generated CycloneDX documents against the official 1.6
   schema; keep the remaining whole-artifact acceptance requirements in scope.

```sh
GOTOOLCHAIN=go1.26.7 GOWORK=off GOFLAGS=-mod=readonly \
  go test -race ./scripts/go-licenses
make image-license-test
```

The classifier is an aid, not the review result: its CSV command can exit zero
while emitting Unknown findings. Segmentio's MIT-0 resolution is bound to its
reviewed module/version/sum and exact LICENSE bytes; there is no blanket Unknown
exception.
