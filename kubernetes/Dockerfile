# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS build
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
RUN --mount=type=cache,target=/root/.cache/go-build go build -trimpath -buildvcs=false -o /tmp/configra-go-licenses ./scripts/go-licenses
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH sh scripts/go-notices/collect.sh /out/licenses/go1.26.7
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/configra-kubernetes ./cmd/configra-kubernetes
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    /tmp/configra-go-licenses -binary /out/configra-kubernetes -out /out/licenses/configra-kubernetes-modules
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/configra-kubernetes /configra-kubernetes
COPY --from=build /out/licenses/ /licenses/configra-kubernetes/
USER 65532:65532
ENTRYPOINT ["/configra-kubernetes"]
