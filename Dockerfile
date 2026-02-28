# ============================================================================
# HiveMind — Multi-stage production build
# Output: ~25MB scratch image with single static binary
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

# Stage 3: Production image
FROM scratch
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go-builder /hivemind /hivemind
COPY deploy/production/config.yaml /etc/hivemind/config.yaml

EXPOSE 8080
EXPOSE 50051
EXPOSE 51820/udp

ENTRYPOINT ["/hivemind"]
CMD ["serve", "--config", "/etc/hivemind/config.yaml"]
