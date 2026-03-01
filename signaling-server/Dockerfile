# Lightweight standalone signaling server image.
# Build:   docker build -t hivemind-signaling -f signaling-server/Dockerfile .
# Run:     docker run -p 7777:7777 hivemind-signaling

FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /signaling-server \
    ./signaling-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /signaling-server /signaling-server

EXPOSE 7777

ENTRYPOINT ["/signaling-server"]
CMD ["--port", "7777"]
