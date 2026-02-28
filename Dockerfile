# ============================================================================
# HiveMind — Production Build (Go + Python Worker + CUDA)
# ============================================================================
# Supports GPU (NVIDIA CUDA) and CPU-only modes.
# CPU-only: set CUDA_VISIBLE_DEVICES="" in the container environment.
# Mock mode: set HIVEMIND_MOCK=true to use mock inference (no Python worker).
# ============================================================================

# Stage 1: Build web dashboard
FROM node:20-alpine AS web-builder
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --production=false
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /build/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /hivemind \
    ./cmd/hivemind

# Stage 3: Runtime with Python + torch + CUDA support
FROM nvidia/cuda:12.4.0-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip ca-certificates curl && \
    rm -rf /var/lib/apt/lists/* && \
    ln -sf /usr/bin/python3 /usr/bin/python

# Copy Go binary and config
COPY --from=go-builder /hivemind /hivemind
COPY --from=go-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY deploy/production/config.yaml /etc/hivemind/config.yaml

# Copy Python worker
COPY worker/ /app/worker/
WORKDIR /app

# Install Python dependencies (torch with CUDA support)
RUN pip3 install --no-cache-dir \
    grpcio>=1.60.0 \
    grpcio-tools>=1.60.0 \
    protobuf>=4.25.0 \
    structlog>=24.1.0 \
    torch>=2.1.0 \
    transformers>=4.36.0 \
    accelerate>=0.25.0 \
    safetensors>=0.4.0 \
    psutil>=5.9.0 \
    sentencepiece

ENV WORKER_PORT=50051

EXPOSE 8080
EXPOSE 50051
EXPOSE 51820/udp

ENTRYPOINT ["/hivemind"]
CMD ["serve", "--config", "/etc/hivemind/config.yaml"]
