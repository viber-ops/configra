# Reviewed system CA material

This collector handles the public system trust store copied from the pinned Go
builder. It does not read or manage Configra-issued CA keys or workload keys.
The current profile is Debian `ca-certificates 20250419~deb12u2`; amd64 and arm64
builder variants were checked to have identical bundle, selection and legal text.

Docker downloads the exact source archive from the content-addressed
[Debian Snapshot URL](https://snapshot-cloudflare.debian.org/file/fde2c52fcb2eca9b91939d726dcce3cc6b6def90)
with `ADD --checksum`, then invokes the collector with the builder filesystem and
that archive. The live security archive can remove superseded versions; the
snapshot retains the reviewed bytes. The SHA-1 in the snapshot URL only locates
the file; both Docker and the collector still verify the original SHA-256
`934942a985b2efd7a8c122cb1d42cd87911cf6ac365c545938619fcf268ea8f4`. Only the
download location changed: the package version, certificate bytes and license
materials are the same reviewed inputs.

The collector verifies every reviewed SHA-256 before publishing a new output
directory. Changed source, trust data or notices need review; existing output is
never silently replaced.

The unchanged source archive preserves Mozilla's MPL-covered certificate inputs
and Debian's GPL-covered generation/packaging scripts, with both license texts.
Only the resulting certificate bundle is installed at the runtime trust path;
the archived scripts are source material, not installed runtime commands. See
[the primary-source review](../../docs/research/distribution-license-requirements.md)
for the legal scopes and source identities.

```sh
# From the source checkout:
GOTOOLCHAIN=go1.26.10 GOWORK=off GOFLAGS=-mod=readonly \
  go test -race ./scripts/ca-notices
make image-license-test
```

Changing the builder or system trust policy requires checking the new package,
source archive, selected certificate bytes, copyright and complete license texts
before updating the hash-bound profile. Do not update hashes solely to silence a
build failure. This collection does not establish production operational readiness.
