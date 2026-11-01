# syntax=docker/dockerfile:1.7
#
# Coffer image. Stages (DEVELOPMENT.md section 7):
#   test   full toolchain with gcc (the race detector needs cgo) plus pinned analysis tools
#   build  static CGO_ENABLED=0 binary, version stamped in
#   final  distroless, non-root, nothing but the binary
# A `web` stage (the SvelteKit bundle) joins when the frontend exists.
#
# Base images are pinned by tag for now. Pin by digest when the publish job is added (Phase 4).

ARG GO_VERSION=1.27.3

# ---- test -------------------------------------------------------------------------------------
FROM golang:${GO_VERSION}-bookworm AS test
ENV CGO_ENABLED=1 \
    GOFLAGS=-buildvcs=false \
    GOTOOLCHAIN=local
# Analysis tools are pinned here, baked into the image, and never part of go.mod.
RUN go install honnef.co/go/tools/cmd/staticcheck@v0.8.2 \
 && go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
# compose.test.yml bind-mounts the live source over /src; this COPY is for plain `docker build`.
COPY . .
CMD ["go", "test", "-race", "-count=1", "./..."]

# ---- build ------------------------------------------------------------------------------------
FROM golang:${GO_VERSION}-bookworm AS build
ARG VERSION=dev
ENV CGO_ENABLED=0 \
    GOFLAGS=-trimpath \
    GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build \
      -ldflags "-s -w -X github.com/AutumnsGrove/Coffer/internal/version.stamp=${VERSION}" \
      -o /out/coffer ./cmd/coffer

# ---- final ------------------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian13:nonroot AS final
COPY --from=build /out/coffer /coffer
USER nonroot:nonroot
# Production port, loopback only on the potato: compose publishes 127.0.0.1:8894 and
# `tailscale serve` proxies it. 8894 is reserved for Coffer (8893 is Cairn, 8892 is Warbler).
EXPOSE 8894
ENTRYPOINT ["/coffer"]
