# HiveMind

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19+-61DAFB?style=for-the-badge&logo=react&logoColor=black)](https://react.dev)
[![gRPC](https://img.shields.io/badge/gRPC-Protobuf-244c5a?style=for-the-badge&logo=grpc)](https://grpc.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/JohnPitter/hivemind/ci.yml?branch=master&style=for-the-badge&label=CI)](https://github.com/JohnPitter/hivemind/actions)

**Distributed P2P Cooperative AI Inference**

*Run large AI models across multiple machines over the internet via tensor parallelism*

[Quick Start](#quick-start) •
[Features](#features) •
[Architecture](#architecture) •
[CLI](#cli) •
[Dashboard](#dashboard) •
[Deployment](#deployment)

</div>

---

## Overview

HiveMind lets you run AI models that don't fit on a single GPU by splitting them across multiple machines connected over the internet. Think of it as **BitTorrent for AI inference** — peers contribute their GPU power and the model runs cooperatively.

**How it works:**

1. One peer **creates a room** and picks a model
2. Others **join with an invite code** via WireGuard mesh
3. Model layers are **distributed by VRAM** — more powerful GPUs get more layers
4. Inference runs as a **forward-pass chain** — tensors flow peer-to-peer with zstd compression
5. The API is **OpenAI-compatible** — plug in any client that speaks `/v1/chat/completions`

---

## Quick Start

### Requirements

| Requirement | Version |
|---|---|
| Go | 1.25+ |
| Node.js | 20+ |
| Python | 3.11+ |
| GPU | NVIDIA with CUDA (recommended) |

### Build from Source

```bash
git clone https://github.com/JohnPitter/hivemind.git
cd hivemind
make build
```

This builds the React dashboard and compiles everything into a single Go binary at `bin/hivemind`.

### Create a Room

```bash
# Start a room with a model
./bin/hivemind create

# You'll get an invite code like: a8f3k2m9x1b4
```

### Join a Room

```bash
# On another machine
./bin/hivemind join a8f3k2m9x1b4
```

### Start Chatting

```bash
# Interactive chat in the terminal
./bin/hivemind chat
```

Or use any OpenAI-compatible client:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

**That's it!** The model runs distributed across all peers in the room.

---

## Features

| Feature | Description |
|---|---|
| **Tensor Parallelism** | Split model layers across GPUs proportional to VRAM |
| **WireGuard Mesh** | Encrypted P2P networking with NAT traversal |
| **OpenAI-Compatible API** | `/v1/chat/completions`, `/v1/images/generations`, `/v1/models` |
| **Interactive CLI** | Create, join, chat, status — all from the terminal |
| **Web Dashboard** | Real-time monitoring with peer stats, VRAM usage, layer map |
| **Streaming SSE** | Token-by-token streaming for chat completions |
| **zstd Compression** | ~40% tensor size reduction for faster transfers |
| **Circuit Breaker** | Per-peer fault isolation with automatic recovery |
| **Leader Election** | Automatic failover when the host goes down |
| **Diffusion Pipeline** | Image generation with 3-stage pipeline parallelism |
| **Health Monitoring** | Periodic gRPC health checks with EMA latency tracking |
| **Adaptive Timeout** | Timeout adjusts dynamically based on peer latency |
| **Single Binary** | Dashboard embedded — one file to deploy |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI (cobra + lipgloss)                │
├─────────────────────────────────────────────────────────────┤
│                    HTTP API (chi router)                      │
│  /v1/chat/completions  /v1/images/generations  /v1/models    │
├───────────────┬─────────────────────┬───────────────────────┤
│   handlers/   │     services/       │       infra/          │
│  inference    │  distributed_infer  │  peer_service         │
│  room         │  resilience         │  wireguard            │
│  health       │  leader_election    │  circuit_breaker      │
│  web (SPA)    │  diffusion_pipeline │  tensor_compress      │
│               │  metrics            │  signaling            │
│               │                     │  worker_manager       │
├───────────────┴─────────────────────┴───────────────────────┤
│                    gRPC (protobuf)                            │
│              Go ←→ Python Worker  |  Peer ←→ Peer            │
├─────────────────────────────────────────────────────────────┤
│              WireGuard Mesh (10.42.0.0/16)                   │
└─────────────────────────────────────────────────────────────┘
```

### Clean Architecture Layers

| Layer | Responsibility | Example |
|---|---|---|
| **handlers/** | HTTP request/response, validation | `inference.go`, `room.go` |
| **services/** | Business logic, orchestration | `distributed_inference.go`, `resilience.go` |
| **infra/** | External systems, networking, I/O | `peer_service.go`, `wireguard.go` |

Dependencies flow inward: `handlers → services → infra`. Interfaces define boundaries.

### Tech Stack

| Component | Technology |
|---|---|
| **Backend** | Go 1.25, chi router, slog |
| **Frontend** | React 19, TypeScript, Tailwind CSS, Vite |
| **CLI** | cobra, lipgloss |
| **Networking** | WireGuard, gRPC + Protobuf |
| **Compression** | zstd (klauspost/compress) |
| **Config** | Viper (YAML + env vars) |
| **AI Runtime** | Python, transformers, diffusers |
| **Deployment** | Docker (scratch), nginx, systemd |

---

## CLI

The CLI uses an amber/honey bee theme with styled terminal output.

```
hivemind create              Create a new room (interactive model picker)
hivemind join <code>         Join a room with an invite code
hivemind chat                Interactive chat with streaming output
hivemind status              Room status: peers, VRAM, layers, latency
hivemind leave               Leave the current room
hivemind stop                Stop hosting and dissolve the room
hivemind web                 Open the web dashboard on port 3000
hivemind serve               Start the HTTP API server
```

---

## Dashboard

The web dashboard is embedded into the Go binary and served as a SPA. Access it at `http://localhost:3000` after running `hivemind web`.

| Panel | Shows |
|---|---|
| **Peers** | GPU info, VRAM bars, latency per peer |
| **Resources** | CPU, RAM, VRAM, disk usage |
| **Layer Map** | Visual distribution of model layers across peers |
| **Distributed** | Tensor transfers, compression ratio, forward pass latency |
| **Chat** | Interactive playground with streaming responses |

---

## API

OpenAI-compatible endpoints — works with any client that supports the OpenAI format.

### Chat Completion

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain tensor parallelism"}
    ],
    "stream": true,
    "max_tokens": 512
  }'
```

### Image Generation

```bash
curl http://localhost:8080/v1/images/generations \
  -H "Content-Type: application/json" \
  -d '{
    "model": "stable-diffusion-xl",
    "prompt": "A swarm of golden bees building a neural network",
    "size": "1024x1024"
  }'
```

### All Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/chat/completions` | Chat completion (streaming + non-streaming) |
| `POST` | `/v1/images/generations` | Image generation via diffusion pipeline |
| `GET` | `/v1/models` | List available models |
| `POST` | `/room/create` | Create a new room |
| `POST` | `/room/join` | Join with invite code |
| `DELETE` | `/room/leave` | Leave the current room |
| `GET` | `/room/status` | Room status with peer info |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Prometheus-compatible metrics |

---

## Configuration

### Default Configuration

```yaml
api:
  host: "127.0.0.1"
  port: 8080
  rate_limit: 60          # requests per second per IP
  max_body_bytes: 10485760 # 10MB

room:
  max_peers: 10
  auto_approve: true
  invite_length: 12

worker:
  grpc_port: 50051
  health_interval_s: 5
  max_restarts: 3

log:
  level: "info"           # debug, info, warn, error
  format: "text"          # text or json
```

Config file location: `~/.hivemind/config.yaml`

### Environment Variables

All config values can be overridden with `HIVEMIND_` prefix:

```bash
HIVEMIND_API_PORT=9090
HIVEMIND_LOG_LEVEL=debug
HIVEMIND_LOG_FORMAT=json
HIVEMIND_ROOM_MAX_PEERS=20
```

---

## Deployment

### Docker

```bash
docker build -t hivemind .
docker run -p 8080:8080 --gpus all hivemind
```

The multi-stage Dockerfile produces a ~25MB scratch image with a single static binary.

### Production Stack (1,000 users)

The `deploy/production/` directory contains a complete production setup:

```bash
cd deploy/production
./deploy.sh up
```

| File | Purpose |
|---|---|
| `docker-compose.yml` | nginx + 2x hivemind (GPU) + redis |
| `nginx.conf` | Load balancer with tiered rate limiting, TLS, SSE |
| `config.yaml` | Tuned for 1k concurrent users |
| `hivemind.service` | systemd unit with security hardening |
| `env.example` | Environment variable template |
| `deploy.sh` | Deploy script: up/down/restart/logs/status/scale |

### Production Sizing

| Resource | Recommendation |
|---|---|
| **CPU** | 4+ vCPU per instance |
| **RAM** | 16GB minimum |
| **GPU** | NVIDIA with 8GB+ VRAM per instance |
| **Network** | 1 Gbps (tensor transfers are bandwidth-intensive) |
| **Storage** | 100GB SSD |

---

## Development

```bash
# Build everything
make build

# Run tests (Go + Python)
make test

# Run Go tests with race detector
make test-go

# Generate protobuf code
make proto

# Lint
make lint

# Clean build artifacts
make clean
```

### Project Structure

```
hivemind/
├── cmd/hivemind/          # CLI entry point
├── internal/
│   ├── api/               # HTTP server, middleware
│   ├── cli/               # CLI commands (cobra + lipgloss)
│   ├── config/            # Viper configuration
│   ├── handlers/          # HTTP handlers (inference, room, health)
│   ├── infra/             # WireGuard, gRPC peers, circuit breaker
│   ├── logger/            # Structured logging (slog)
│   ├── models/            # Domain types (Room, Peer, Inference)
│   └── services/          # Business logic (distributed inference, resilience)
├── proto/                 # Protobuf definitions
├── web/                   # React + TypeScript dashboard
├── worker/                # Python AI worker (transformers, diffusers)
├── deploy/production/     # Docker, nginx, systemd configs
├── Dockerfile             # Multi-stage scratch build
└── Makefile               # Build, test, lint, proto targets
```

---

## Roadmap

- [x] Phase 1 — Scaffolding + Core Types
- [x] Phase 2 — Interactive CLI
- [x] Phase 3 — Web Dashboard
- [x] Phase 4 — HTTP API + Single-Node Inference
- [x] Phase 5 — P2P + WireGuard Mesh
- [x] Phase 6 — Tensor Parallelism
- [x] Phase 7 — Resilience + Image Models
- [x] Phase 8 — CI/CD + Production Deployment

---

## License

MIT License — see [LICENSE](LICENSE) file.

---

## Contributing

Contributions are welcome! The `master` branch is protected:

1. Fork the repository
2. Create a feature branch
3. Submit a pull request (requires 1 approval + CI passing)

---

## Support

- **Issues:** [GitHub Issues](https://github.com/JohnPitter/hivemind/issues)
- **Discussions:** [GitHub Discussions](https://github.com/JohnPitter/hivemind/discussions)
