# Dockerfile — TellCartman gRPC server
#
# Multi-stage build:
#   Stage 1 (builder): compiles the Go binary with full Go toolchain
#   Stage 2 (runtime): minimal distroless image — no shell, no package manager
#
# Why distroless?
#   - Smaller image (~5MB vs ~200MB for alpine)
#   - No shell means no risk of shell injection attacks
#   - Only the Go binary and its dependencies are included
#
# Build: docker build -t tc-server:latest .
# Run:   docker run -p 50051:50051 -p 8080:8080 \
#          -e QUEUE_URL=... -e BUCKET_NAME=... tc-server:latest

# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# Install git (needed for go mod download with VCS deps)
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go.mod and go.sum first so Docker caches the dependency layer.
# Dependencies only change when go.mod/go.sum change, not on every code edit.
COPY go.mod go.sum ./
RUN go mod download

# Copy source after dependencies are cached
COPY . .

# Build the server binary
# -trimpath: removes local file paths from binary (security + reproducibility)
# CGO_ENABLED=0: static binary, no C dependencies — required for distroless
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /build/tc-server ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

# Copy CA certs from builder (needed for HTTPS calls to AWS APIs)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled binary
COPY --from=builder /build/tc-server /tc-server

# gRPC port
EXPOSE 50051
# HTTP health check port
EXPOSE 8080

# Run as non-root (UID 65532 = nonroot in distroless)
USER nonroot:nonroot

# The binary reads all config from environment variables
ENTRYPOINT ["/tc-server"]
