# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 AS build
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOWORK=off
WORKDIR /src/kubernetes
COPY kubernetes/go.mod kubernetes/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY kubernetes/internal ./internal
COPY kubernetes/cmd ./cmd
COPY kubernetes/LICENSE kubernetes/NOTICE /out/licenses/
COPY scripts/go-notices ./scripts/go-notices
COPY scripts/go-licenses ./scripts/go-licenses
COPY scripts/ca-notices ./scripts/ca-notices
ADD --checksum=sha256:b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d https://security.debian.org/debian-security/pool/updates/main/c/ca-certificates/ca-certificates_20250419~deb12u1.tar.xz /tmp/ca-certificates-source.tar.xz
RUN --mount=type=cache,target=/root/.cache/go-build go run ./scripts/ca-notices -rootfs / -source /tmp/ca-certificates-source.tar.xz -out /out/licenses/ca-certificates
RUN --mount=type=cache,target=/root/.cache/go-build go build -trimpath -buildvcs=false -o /tmp/configra-go-licenses ./scripts/go-licenses
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH sh scripts/go-notices/collect.sh /out/licenses/go1.26.7
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/configra-kubernetes ./cmd/configra-kubernetes
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    /tmp/configra-go-licenses -binary /out/configra-kubernetes -out /out/licenses/configra-kubernetes-modules
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS package
COPY scripts/compose-distribution.mjs /compose-distribution.mjs
COPY --from=build /out/configra-kubernetes /artifact/configra-kubernetes
COPY --from=build /out/licenses/ /artifact/licenses/configra-kubernetes/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /artifact/etc/ssl/certs/ca-certificates.crt
RUN node /compose-distribution.mjs image /artifact configra-kubernetes
FROM scratch
COPY --from=package /artifact/ /
USER 65532:65532
ENTRYPOINT ["/configra-kubernetes"]
