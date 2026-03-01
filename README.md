# HiveMind

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19+-61DAFB?style=for-the-badge&logo=react&logoColor=black)](https://react.dev)
[![gRPC](https://img.shields.io/badge/gRPC-Protobuf-244c5a?style=for-the-badge&logo=grpc)](https://grpc.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/JohnPitter/hivemind/ci.yml?branch=master&style=for-the-badge&label=CI)](https://github.com/JohnPitter/hivemind/actions)
[![Version](https://img.shields.io/badge/Version-1.0.0-F97316?style=for-the-badge)](CHANGELOG.md)

**Distributed P2P Cooperative AI Inference**

*Run large AI models across multiple machines over the internet via tensor parallelism*

[Quick Start](#quick-start) •
[How It Works](#how-it-works) •
[Features](#features) •
[Architecture](#architecture) •
[CLI](#cli) •
[API](#api) •
[Configuration](#configuration) •
[Deployment](#deployment) •
[Development](#development)

</div>

---

## Overview

HiveMind lets you run AI models that don't fit on a single GPU by splitting them across multiple machines connected over the internet. Think of it as **BitTorrent for AI inference** — peers contribute their GPU power and the model runs cooperatively.

```
                    ┌─────────────┐
                    │  Signaling  │    Room discovery + WG key exchange
                    │   Server    │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
         ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
         │ Peer A  │  │ Peer B  │  │ Peer C  │
         │ RTX4090 │──│ RTX3080 │──│ RTX3060 │   WireGuard Mesh
         │ L0-L24  │  │ L25-L40 │  │ L41-L48 │   Layers by VRAM
         └─────────┘  └─────────┘  └─────────┘
```

---

## How It Works

1. **Start a signaling server** — lightweight rendezvous for room discovery and WireGuard key exchange
2. One peer **creates a room** and picks a model (e.g., Llama 3 70B)
3. Others **join with an invite code** — a WireGuard mesh is established automatically
4. Model layers are **distributed by VRAM** — more powerful GPUs get more layers
5. Inference runs as a **forward-pass chain** — tensors flow peer-to-peer with zstd compression
6. The API is **OpenAI-compatible** — plug in any client that speaks `/v1/chat/completions`

---

## Quick Start

### Requirements

| Requirement | Version | Notes |
|---|---|---|
| Go | 1.25+ | Backend + CLI |
| Node.js | 20+ | Web dashboard build |
| Python | 3.11+ | AI worker (transformers, diffusers) |
| GPU | NVIDIA with CUDA | Recommended; CPU-only mode available |

### Build from Source

```bash
git clone https://github.com/JohnPitter/hivemind.git
cd hivemind
make build
```

This builds the React dashboard and compiles everything into a single Go binary at `bin/hivemind`.

### Single-Machine Quick Start (Mock Mode)

For testing without GPU or multiple machines:

```bash
# Start in mock mode — no Python worker, no GPU required
HIVEMIND_MOCK=true ./bin/hivemind serve

# In another terminal:
curl -X POST http://localhost:8080/room/create \
  -H "Content-Type: application/json" \
  -d '{"model_id": "TinyLlama/TinyLlama-1.1B"}'

# Chat
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "mock", "messages": [{"role": "user", "content": "Hello!"}]}'

# Open dashboard at http://localhost:8080
```

### Multi-Machine P2P Setup

**Step 1: Start the signaling server** (on any reachable machine)

```bash
./bin/hivemind signaling --port 7777
```

**Step 2: Peer A creates a room**

```bash
# Set signaling server URL
export HIVEMIND_SIGNALING_URL=http://signaling-host:7777

# Start the API server
./bin/hivemind serve --port 8080

# Create a room (from another terminal or curl)
curl -X POST http://localhost:8080/room/create \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "meta-llama/Llama-3-70B-Instruct",
    "model_type": "llm",
    "max_peers": 4,
    "resources": {
      "gpu_name": "NVIDIA RTX 4090",
      "vram_total_mb": 24576,
      "vram_free_mb": 20480,
      "ram_total_mb": 65536,
      "ram_free_mb": 49152,
      "cuda_available": true,
      "platform": "Linux"
    }
  }'

# Response includes an invite_code, e.g. "a8f3k2m9x1b4"
```

**Step 3: Peer B joins the room**

```bash
export HIVEMIND_SIGNALING_URL=http://signaling-host:7777

./bin/hivemind serve --port 8080

curl -X POST http://localhost:8080/room/join \
  -H "Content-Type: application/json" \
  -d '{
    "invite_code": "a8f3k2m9x1b4",
    "resources": {
      "gpu_name": "NVIDIA RTX 3080",
      "vram_total_mb": 10240,
      "vram_free_mb": 8192,
      "ram_total_mb": 32768,
      "ram_free_mb": 24576,
      "cuda_available": true,
      "platform": "Linux"
    }
  }'
```

**Step 4: Chat via any peer's API**

```bash
curl http://peer-a:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3-70B-Instruct",
    "messages": [{"role": "user", "content": "Explain tensor parallelism"}],
    "stream": true,
    "max_tokens": 512
  }'
```

### Using the Interactive CLI

```bash
# Interactive model picker with lipgloss-styled output
./bin/hivemind create

# Join with invite code (animated connection steps)
./bin/hivemind join a8f3k2m9x1b4

# Interactive chat with streaming
./bin/hivemind chat

# Room status with VRAM bars, peer table, layer map
./bin/hivemind status
```

### Docker Quick Start

```bash
# Single node
docker build -t hivemind .
docker run -p 8080:8080 --gpus all hivemind

# P2P test with 3 containers (signaling + 2 peers)
docker compose -f docker-compose.p2p.yml up --build
```

---

## Features

| Feature | Description |
|---|---|
| **Tensor Parallelism** | Split model layers across GPUs proportional to VRAM |
| **WireGuard Mesh** | Encrypted P2P networking with automatic key exchange |
| **Signaling Server** | Lightweight room discovery and WG key exchange (runs as subcommand or standalone) |
| **OpenAI-Compatible API** | `/v1/chat/completions`, `/v1/images/generations`, `/v1/models` |
| **Interactive CLI** | Create, join, chat, status — all from the terminal with lipgloss styling |
| **Web Dashboard** | Real-time monitoring with live peer stats, VRAM usage, layer map, chat playground |
| **Streaming SSE** | Token-by-token streaming for chat completions |
| **API Key Auth** | Bearer token authentication for inference and room management endpoints |
| **zstd Compression** | ~40% tensor size reduction for faster peer-to-peer transfers |
| **Circuit Breaker** | Per-peer fault isolation with automatic recovery |
| **Leader Election** | Automatic failover when the host goes down |
| **Layer Redistribution** | Automatic re-assignment when peers join or leave |
| **Diffusion Pipeline** | Image generation with 3-stage pipeline parallelism (TextEncoder → UNet → VAE) |
| **Health Monitoring** | Periodic gRPC health checks with EMA latency tracking |
| **Adaptive Timeout** | Timeout adjusts dynamically based on peer latency |
| **Model Catalog** | 20 pre-configured models across 5 types (LLM, code, diffusion, multimodal, embedding) |
| **Resource Donation** | Control how much VRAM to contribute via `donation_pct` |
| **Single Binary** | Dashboard embedded — one file to deploy |
| **Mock Mode** | Full mock services for development and testing without GPU |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI (cobra + lipgloss)                │
├─────────────────────────────────────────────────────────────┤
│                    HTTP API (chi router)                      │
│  /v1/chat/completions  /v1/images/generations  /v1/models    │
│                   auth · rate-limit · cors                   │
├───────────────┬─────────────────────┬───────────────────────┤
│   handlers/   │     services/       │       infra/          │
│  inference    │  real_room          │  peer_service         │
│  room         │  real_inference     │  peer_grpc_server     │
│  health       │  distributed_infer  │  wireguard            │
│  catalog      │  resilience         │  circuit_breaker      │
│  web (SPA)    │  leader_election    │  tensor_compress      │
│               │  diffusion_pipeline │  signaling            │
│               │  metrics            │  signaling_client     │
│               │                     │  worker_manager       │
│               │                     │  health_monitor       │
├───────────────┴─────────────────────┴───────────────────────┤
│                    gRPC (protobuf)                            │
│              Go ←→ Python Worker  |  Peer ←→ Peer            │
├─────────────────────────────────────────────────────────────┤
│              WireGuard Mesh (10.42.0.0/16)                   │
└─────────────────────────────────────────────────────────────┘
```

### Clean Architecture Layers

| Layer | Responsibility | Key Files |
|---|---|---|
| **handlers/** | HTTP request/response, validation | `inference.go`, `room.go`, `catalog.go` |
| **services/** | Business logic, orchestration | `real_room.go`, `real_inference.go`, `resilience.go` |
| **infra/** | External systems, networking, I/O | `peer_service.go`, `wireguard.go`, `signaling.go` |

Dependencies flow inward: `handlers → services → infra`. Interfaces define boundaries.

### Service Modes

| Mode | How to Enable | What Runs |
|---|---|---|
| **Real** | Default (no env var) | Signaling client, WireGuard, PeerRegistry, Python worker, gRPC |
| **Mock** | `HIVEMIND_MOCK=true` | In-memory mock services, no GPU/Python required |

### Tech Stack

| Component | Technology |
|---|---|
| **Backend** | Go 1.25, chi router, slog |
| **Frontend** | React 19, TypeScript, Tailwind CSS, Vite |
| **CLI** | cobra, lipgloss, bubbletea |
| **Networking** | WireGuard, gRPC + Protobuf |
| **Compression** | zstd (klauspost/compress) |
| **Config** | Viper (YAML + env vars) |
| **AI Runtime** | Python, transformers, diffusers, torch |
| **Deployment** | Docker (multi-stage), nginx, systemd |

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
hivemind serve               Start the HTTP API + dashboard server
hivemind web                 Open the web dashboard on port 3000
hivemind signaling           Run the signaling/rendezvous server
hivemind health              Health check (for Docker/systemd)
hivemind version             Print version
```

### Global Flags

```
--config string   Config file path (default: ~/.hivemind/config.yaml)
-v, --verbose     Enable debug logging
```

---

## API

OpenAI-compatible endpoints — works with any client that supports the OpenAI format.

### Authentication

Set `HIVEMIND_API_KEY` to enable Bearer token auth:

```bash
export HIVEMIND_API_KEY=my-secret-key

curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer my-secret-key"
```

When `HIVEMIND_API_KEY` is empty (default), auth is disabled for local development.

Protected routes: `/v1/*`, `/room/*`. Public routes: `/health`, `/metrics`, `/api/*`, `/*` (SPA).

### Chat Completion

```bash
# Non-streaming
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3-70B-Instruct",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain tensor parallelism"}
    ],
    "max_tokens": 512
  }'

# Streaming (SSE)
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3-70B-Instruct",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Image Generation

```bash
curl http://localhost:8080/v1/images/generations \
  -H "Content-Type: application/json" \
  -d '{
    "model": "stabilityai/stable-diffusion-xl-base-1.0",
    "prompt": "A swarm of golden bees building a neural network",
    "size": "1024x1024"
  }'
```

### Room Management

```bash
# Create a room
curl -X POST http://localhost:8080/room/create \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "meta-llama/Llama-3-70B-Instruct",
    "model_type": "llm",
    "max_peers": 4,
    "resources": {
      "gpu_name": "NVIDIA RTX 4090",
      "vram_total_mb": 24576,
      "vram_free_mb": 20480,
      "ram_total_mb": 65536,
      "ram_free_mb": 49152,
      "cuda_available": true,
      "platform": "Linux",
      "donation_pct": 80
    }
  }'

# Join a room
curl -X POST http://localhost:8080/room/join \
  -H "Content-Type: application/json" \
  -d '{
    "invite_code": "a8f3k2m9x1b4",
    "resources": { ... }
  }'

# Room status
curl http://localhost:8080/room/status

# Leave
curl -X DELETE http://localhost:8080/room/leave
```

### Model Catalog

```bash
# List all 20 pre-configured models
curl http://localhost:8080/v1/models/catalog

# Get suggestion by available VRAM
curl http://localhost:8080/v1/models/catalog?vram_mb=24000
```

### All Endpoints

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/v1/chat/completions` | Chat completion (streaming + non-streaming) | Yes |
| `POST` | `/v1/images/generations` | Image generation via diffusion pipeline | Yes |
| `GET` | `/v1/models` | List models in current room | Yes |
| `GET` | `/v1/models/catalog` | Browse 20 pre-configured models | Yes |
| `POST` | `/room/create` | Create a new room | Yes |
| `POST` | `/room/join` | Join with invite code | Yes |
| `DELETE` | `/room/leave` | Leave the current room | Yes |
| `GET` | `/room/status` | Room status with peer info | Yes |
| `GET` | `/health` | Health check | No |
| `GET` | `/metrics` | Inference and peer metrics | No |
| `GET` | `/api/room/status` | Dashboard room status (JSON) | No |
| `GET` | `/api/health` | Dashboard health (JSON) | No |
| `GET` | `/*` | Web dashboard (SPA) | No |

---

## Dashboard

The web dashboard is embedded into the Go binary and served as a SPA.

- **Via `serve`:** `http://localhost:8080` (shares port with API)
- **Via `web`:** `http://localhost:3000` (standalone, auto-opens browser)

| Panel | Shows |
|---|---|
| **Peers** | GPU info, VRAM bars, latency per peer |
| **Resources** | CPU, RAM, VRAM, disk usage |
| **Layer Map** | Visual distribution of model layers across peers |
| **Distributed** | Tensor transfers, compression ratio, forward pass latency |
| **Chat** | Interactive playground with real SSE streaming |
| **Room Info** | Room ID, model, invite code, leave/stop buttons |

When no room is active, the dashboard shows instructions for creating or joining a room.

---

## Configuration

### Config File

Default location: `~/.hivemind/config.yaml`

```yaml
api:
  host: "127.0.0.1"
  port: 8080
  rate_limit: 60           # requests per second per IP
  max_body_bytes: 10485760 # 10MB

room:
  max_peers: 10
  auto_approve: true
  invite_length: 12

worker:
  grpc_port: 50051
  health_interval_s: 5
  max_restarts: 3

signaling:
  url: "http://localhost:7777"
  port: 7777

mesh:
  wireguard_port: 51820
  grpc_port: 50052
  config_dir: "~/.hivemind/wg"

peer:
  id: ""                   # auto-generated if empty
  name: ""
  endpoint: ""             # public IP:port for WireGuard

resilience:
  max_retries: 3
  health_interval_s: 5
  circuit_max_fails: 3

log:
  level: "info"            # debug, info, warn, error
  format: "text"           # text or json
```

### Environment Variables

All config values can be overridden with the `HIVEMIND_` prefix:

| Variable | Description | Default |
|---|---|---|
| `HIVEMIND_MOCK` | Enable mock mode (no GPU/Python) | `false` |
| `HIVEMIND_API_KEY` | Bearer token for API auth (empty = disabled) | `` |
| `HIVEMIND_API_CORS_ORIGINS` | Comma-separated allowed CORS origins | `http://localhost:5173,http://localhost:8080` |
| `HIVEMIND_SIGNALING_URL` | Signaling server URL | `http://localhost:7777` |
| `HIVEMIND_PEER_ID` | Override auto-generated peer ID | auto |
| `HIVEMIND_PEER_ENDPOINT` | Public WireGuard endpoint (IP:port) | auto-detect |
| `HIVEMIND_WORKER_DIR` | Python worker directory | `/app/worker` |
| `HIVEMIND_PYTHON_CMD` | Python executable | `python3` |
| `HIVEMIND_API_PORT` | API server port | `8080` |
| `HIVEMIND_API_HOST` | API bind address | `127.0.0.1` |
| `HIVEMIND_LOG_LEVEL` | Log level | `info` |
| `HIVEMIND_LOG_FORMAT` | Log format (`text` or `json`) | `text` |

---

## Deployment

### Docker (Single Node)

```bash
docker build -t hivemind .
docker run -p 8080:8080 --gpus all hivemind
```

CPU-only mode:

```bash
docker run -p 8080:8080 -e CUDA_VISIBLE_DEVICES="" hivemind
```

Mock mode (no Python/GPU):

```bash
docker run -p 8080:8080 -e HIVEMIND_MOCK=true hivemind
```

### Docker Compose P2P (3 Nodes)

```bash
docker compose -f docker-compose.p2p.yml up --build
```

This starts:
- **signaling** — rendezvous server on port 7777
- **alice** — peer node (room host)
- **bob** — peer node (joins via invite code)
- **p2p-tests** — automated test runner

### Signaling Server (Standalone)

For production deployments where the signaling server runs separately:

```bash
# As a subcommand of the main binary
./bin/hivemind signaling --port 7777

# As a standalone lightweight binary
cd signaling-server
docker build -t hivemind-signaling -f Dockerfile ..
docker run -p 7777:7777 hivemind-signaling
```

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
| **Storage** | 100GB SSD (model cache) |

---

## Development

```bash
# Build everything (web + Go binary)
make build

# Run tests (Go + Python)
make test

# Run Go tests with race detector
make test-go

# Run Python tests
make test-python

# Generate protobuf code
make proto

# Lint (go vet + golangci-lint + ruff)
make lint

# Coverage report
make coverage

# Clean build artifacts
make clean
```

### E2E Tests (Docker)

```bash
# API tests (32 assertions) — mock mode
make test-e2e

# Scenario test: full user flow (76 assertions) — mock mode
make test-e2e-scenario

# Two-user scenario: 2 containers (90+ assertions) — mock mode
make test-e2e-2users

# P2P wiring test: signaling + 2 peers — real mode
make test-e2e-p2p

# Real inference: GPU + CPU resource pooling
make test-e2e-real

# Tear down all test stacks
make test-e2e-down
```

### Project Structure

```
hivemind/
├── cmd/hivemind/              # CLI entry point (composition root)
├── internal/
│   ├── api/                   # HTTP server, middleware, auth
│   ├── catalog/               # Model catalog (20 models, 5 types)
│   ├── cli/                   # CLI commands (cobra + lipgloss)
│   ├── config/                # Viper configuration
│   ├── handlers/              # HTTP handlers (inference, room, health, catalog)
│   ├── infra/                 # WireGuard, gRPC peers, signaling, circuit breaker
│   ├── logger/                # Structured logging (slog)
│   ├── models/                # Domain types (Room, Peer, Inference, Resources)
│   └── services/              # Business logic (real_room, real_inference, resilience)
├── gen/                       # Generated protobuf code (Go)
├── proto/                     # Protobuf definitions (worker.proto, peer.proto)
├── signaling-server/          # Standalone signaling server binary + Dockerfile
├── web/                       # React + TypeScript dashboard
│   └── src/
│       ├── components/        # Sidebar, Header, PeersPanel, ChatPlayground, ...
│       ├── hooks/             # useRoomStatus (polling)
│       └── lib/               # API client (SSE streaming, room management)
├── worker/                    # Python AI worker
│   └── worker/
│       ├── inference/         # LLM engine (transformers) + diffusion engine
│       ├── tensor/            # Tensor serialization (numpy wire format)
│       └── gen/               # Generated protobuf code (Python)
├── tests/e2e/                 # E2E test scripts (curl-based)
├── deploy/production/         # Docker, nginx, systemd configs
├── docker-compose.p2p.yml     # P2P test stack (signaling + 2 peers)
├── docker-compose.test.yml    # E2E test stack (multiple profiles)
├── Dockerfile                 # Multi-stage production build
└── Makefile                   # Build, test, lint, proto, e2e targets
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
- [x] Phase 9 — **v1.0: Distributed P2P Inference (Full Integration)**

### What's Next (v1.1+)

- [ ] Distributed token generation loop (multi-node coordinated generation)
- [ ] `EmbedTokens` gRPC method for distributed embedding
- [ ] NAT traversal improvements (STUN/TURN)
- [ ] Multi-room support per node
- [ ] Web dashboard: room creation/join UI
- [ ] Prometheus metrics exporter
- [ ] Helm chart for Kubernetes deployment

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
