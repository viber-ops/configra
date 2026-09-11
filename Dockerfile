# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts
COPY web/index.html ./
COPY web/src ./src
RUN npm run build

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
COPY --from=web-build /src/web/dist ./web/dist
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/configra ./cmd/configra

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/configra /configra

USER 65532:65532
EXPOSE 8443 9443
ENTRYPOINT ["/configra"]
