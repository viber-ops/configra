# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts
COPY web/index.html ./
COPY web/vite.config.mjs ./
COPY web/scripts/license-plugin.mjs web/scripts/reviewed-licenses.json web/scripts/export-licenses.mjs ./scripts/
COPY web/src ./src
RUN npm run build
RUN node scripts/export-licenses.mjs /src/web/dist /out/ui-licenses

FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY LICENSE NOTICE /out/licenses/
COPY --from=web-build /out/ui-licenses /out/licenses/ui
COPY scripts/go-notices ./scripts/go-notices
COPY scripts/go-licenses ./scripts/go-licenses
COPY scripts/ca-notices ./scripts/ca-notices
ADD --checksum=sha256:b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d https://security.debian.org/debian-security/pool/updates/main/c/ca-certificates/ca-certificates_20250419~deb12u1.tar.xz /tmp/ca-certificates-source.tar.xz
RUN --mount=type=cache,target=/root/.cache/go-build go run ./scripts/ca-notices -rootfs / -source /tmp/ca-certificates-source.tar.xz -out /out/licenses/ca-certificates
RUN --mount=type=cache,target=/root/.cache/go-build go build -trimpath -buildvcs=false -o /tmp/configra-go-licenses ./scripts/go-licenses
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH sh scripts/go-notices/collect.sh /out/licenses/go1.26.7
COPY --from=web-build /src/web/dist ./web/dist
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/configra ./cmd/configra
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    /tmp/configra-go-licenses -binary /out/configra -out /out/licenses/configra-modules

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS package
COPY scripts/compose-distribution.mjs /compose-distribution.mjs
COPY --from=build /out/configra /artifact/configra
COPY --from=build /out/licenses/ /artifact/licenses/configra/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /artifact/etc/ssl/certs/ca-certificates.crt
RUN node /compose-distribution.mjs image /artifact configra

FROM scratch
COPY --from=package /artifact/ /

USER 65532:65532
EXPOSE 8443 9443
ENTRYPOINT ["/configra"]
