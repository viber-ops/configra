# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS build
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOWORK=off
WORKDIR /src/configra/kubernetes
COPY configra-go/go.mod configra-go/go.sum /src/configra-go/
COPY configra-go/*.go /src/configra-go/
COPY configra/kubernetes/go.mod configra/kubernetes/go.sum ./
RUN go mod edit -replace=github.com/viber-ops/configra-go=../../configra-go
COPY configra/kubernetes/internal ./internal
COPY configra/kubernetes/cmd ./cmd
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/configra-kubernetes ./cmd/configra-kubernetes
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/configra-kubernetes /configra-kubernetes
USER 65532:65532
ENTRYPOINT ["/configra-kubernetes"]
