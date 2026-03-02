# =============================================================================
# Dockerfile.builder  –  Full Stratavore build with protobuf / gRPC support
#
# This image installs protoc + Go gRPC plugins and performs a complete build
# that includes generated protobuf stubs.  Use it when you want the binary
# gRPC transport instead of the default HTTP/JSON fallback.
#
# Quick start:
#   docker build -f Dockerfile.builder -t stratavore-builder .
#   docker run --rm -v "$PWD/dist:/dist" stratavore-builder
#
# Or via Compose:
#   docker compose -f docker-compose.builder.yml run --rm builder
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1 – proto-toolchain
# Installs protoc and the Go gRPC plugins into a slim base image.
# Pinning versions here makes builds reproducible.
# -----------------------------------------------------------------------------
FROM golang:1.23-alpine AS proto-toolchain

ARG PROTOC_VERSION=25.3
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN apk add --no-cache \
        curl \
        unzip \
        git \
        make \
        gcc \
        musl-dev

# Install protoc binary
RUN PB_REL="https://github.com/protocolbuffers/protobuf/releases" && \
    ARCHIVE="protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    curl -fsSL "${PB_REL}/download/v${PROTOC_VERSION}/${ARCHIVE}" -o /tmp/protoc.zip && \
    unzip /tmp/protoc.zip -d /usr/local && \
    rm /tmp/protoc.zip && \
    protoc --version

# Install Go protobuf / gRPC code-generation plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0

# Verify everything is on PATH
RUN protoc --version && \
    protoc-gen-go --version && \
    protoc-gen-go-grpc --version

# -----------------------------------------------------------------------------
# Stage 2 – builder
# Generates protobuf stubs and compiles all three Stratavore binaries.
# -----------------------------------------------------------------------------
FROM proto-toolchain AS builder

WORKDIR /build

# Cache module download layer separately from source
COPY go.mod go.sum ./
RUN go mod download

# Copy full source
COPY . .

# Generate protobuf code – errors are fatal in this image (unlike Makefile fallback)
RUN mkdir -p pkg/api/generated && \
    protoc \
        --go_out=pkg/api/generated \
        --go_opt=paths=source_relative \
        --go-grpc_out=pkg/api/generated \
        --go-grpc_opt=paths=source_relative \
        --proto_path=pkg/api \
        pkg/api/stratavore.proto && \
    echo "[OK] protobuf code generated" && \
    ls -lh pkg/api/generated/

# Build all three binaries with version injection
ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT=unknown

RUN BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') && \
    LDFLAGS="-ldflags \"-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}\"" && \
    go build -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}" \
        -o bin/stratavore      ./cmd/stratavore && \
    go build -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}" \
        -o bin/stratavored     ./cmd/stratavored && \
    go build -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}" \
        -o bin/stratavore-agent ./cmd/stratavore-agent && \
    echo "[OK] all binaries built" && \
    ls -lh bin/

# Run tests inside the build image so CI gets a single layer to cache
RUN go test ./... -short -count=1 2>&1 | tee /tmp/test-results.txt || true && \
    echo "[INFO] test results saved to /tmp/test-results.txt"

# -----------------------------------------------------------------------------
# Stage 3 – export
# A scratch-like stage that copies only the compiled artifacts.
# docker run --rm -v "$PWD/dist:/dist" stratavore-builder  copies bins out.
# -----------------------------------------------------------------------------
FROM alpine:3.23 AS export

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/bin/           /dist/bin/
COPY --from=builder /build/pkg/api/generated/ /dist/generated/
COPY --from=builder /tmp/test-results.txt  /dist/test-results.txt

# Default entrypoint: just list what was built so CI logs are informative
CMD ["sh", "-c", "echo '=== Built artifacts ===' && ls -lh /dist/bin/ && echo && echo '=== Generated protobuf files ===' && ls -lh /dist/generated/"]

# -----------------------------------------------------------------------------
# Stage 4 – runtime (daemon only)
# Minimal production image containing the daemon binary.
# This replicates Dockerfile.daemon but with gRPC compiled in.
# -----------------------------------------------------------------------------
FROM alpine:3.23 AS runtime

LABEL org.opencontainers.image.title="stratavored (gRPC build)"
LABEL org.opencontainers.image.description="Stratavore daemon with full gRPC/protobuf support"

RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S stratavore && \
    adduser  -S stratavore -G stratavore

WORKDIR /app

COPY --from=builder /build/bin/stratavored          /usr/local/bin/stratavored
COPY --from=builder /build/configs/stratavore.yaml  /etc/stratavore/stratavore.yaml

RUN mkdir -p /var/lib/stratavore && \
    chown stratavore:stratavore /var/lib/stratavore

USER stratavore

EXPOSE 50051 9091

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://localhost:50051/health || exit 1

ENTRYPOINT ["/usr/local/bin/stratavored"]
