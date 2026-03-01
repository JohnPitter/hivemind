# HiveMind

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19+-61DAFB?style=for-the-badge&logo=react&logoColor=black)](https://react.dev)
[![gRPC](https://img.shields.io/badge/gRPC-Protobuf-244c5a?style=for-the-badge&logo=grpc)](https://grpc.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/JohnPitter/hivemind/ci.yml?branch=master&style=for-the-badge&label=CI)](https://github.com/JohnPitter/hivemind/actions)
[![Version](https://img.shields.io/badge/v1.0.0-F97316?style=for-the-badge)](CHANGELOG.md)

**Distributed P2P Cooperative AI Inference**

*Share GPU power across the internet to run models that don't fit on a single machine*

</div>

---

## What is HiveMind?

HiveMind splits AI models across multiple GPUs connected over the internet. Each peer contributes VRAM, and model layers are distributed proportionally — a peer with 24 GB gets more layers than one with 8 GB. Inference runs as a forward-pass chain: tensors flow peer-to-peer through an encrypted WireGuard mesh with zstd compression.

The API is OpenAI-compatible. Any client that speaks `/v1/chat/completions` works out of the box.

```
         Signaling Server
              │
    ┌─────────┼─────────┐
    │         │         │
 Peer A    Peer B    Peer C
 RTX 4090  RTX 3080  RTX 3060     WireGuard Mesh
 L0–L24    L25–L40   L41–L48     Layers by VRAM
```

---

## Quick Start

### Build

```bash
git clone https://github.com/JohnPitter/hivemind.git && cd hivemind
make build     # builds web dashboard + Go binary → bin/hivemind
```

Requires Go 1.25+, Node.js 20+, Python 3.11+.

### Try It Locally (Mock Mode)

No GPU needed. Mock services simulate the full flow:

```bash
HIVEMIND_MOCK=true ./bin/hivemind serve
# Dashboard: http://localhost:8080
# API:       http://localhost:8080/v1/chat/completions
```

### Run Distributed (2+ Machines)

```bash
# 1. Start the signaling server (any reachable host)
./bin/hivemind signaling --port 7777

# 2. Peer A — create a room
export HIVEMIND_SIGNALING_URL=http://signaling-host:7777
./bin/hivemind serve
./bin/hivemind create   # → prints invite code

# 3. Peer B — join with the invite code
export HIVEMIND_SIGNALING_URL=http://signaling-host:7777
./bin/hivemind serve
./bin/hivemind join <invite-code>

# 4. Chat from any peer
./bin/hivemind chat
```

### Docker

```bash
# Single node with GPU
docker build -t hivemind .
docker run -p 8080:8080 --gpus all hivemind

# P2P test (signaling + 2 peers + automated tests)
docker compose -f docker-compose.p2p.yml up --build
```

---

## CLI

```
hivemind create              Interactive model picker → creates room → prints invite code
hivemind join <code>         Join room via signaling + WireGuard handshake
hivemind chat                Streaming chat in the terminal
hivemind status              Peers, VRAM bars, layer map, latency
hivemind leave / stop        Leave or dissolve the room
hivemind serve               HTTP API + embedded dashboard (port 8080)
hivemind web                 Dashboard-only server (port 3000, auto-opens browser)
hivemind signaling           Run the signaling/rendezvous server (port 7777)
hivemind health              Health check (for Docker/systemd probes)
hivemind version             Print version
```

Flags: `--config <path>` (default `~/.hivemind/config.yaml`), `-v` (debug logging).

---

## API

OpenAI-compatible. Works with any OpenAI client library.

### Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/chat/completions` | Chat (streaming SSE + non-streaming) |
| `POST` | `/v1/images/generations` | Image generation (diffusion pipeline) |
| `GET` | `/v1/models` | Models available in current room |
| `GET` | `/v1/models/catalog` | 20 pre-configured models (filter by `?vram_mb=`) |
| `POST` | `/room/create` | Create room (model_id, resources, max_peers) |
| `POST` | `/room/join` | Join room (invite_code, resources) |
| `DELETE`| `/room/leave` | Leave current room |
| `GET` | `/room/status` | Room state, peers, layers |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Inference + peer metrics |

### Examples

```bash
# Chat
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama-3-70b","messages":[{"role":"user","content":"Hello"}],"stream":true}'

# Image generation
curl http://localhost:8080/v1/images/generations \
  -H "Content-Type: application/json" \
  -d '{"model":"stable-diffusion-xl","prompt":"golden bees building a neural network"}'

# Create room
curl -X POST http://localhost:8080/room/create \
  -H "Content-Type: application/json" \
  -d '{"model_id":"meta-llama/Llama-3-70B-Instruct","max_peers":4}'

# Join room
curl -X POST http://localhost:8080/room/join \
  -H "Content-Type: application/json" \
  -d '{"invite_code":"a8f3k2m9x1b4"}'
```

### Authentication

Set `HIVEMIND_API_KEY` to protect `/v1/*` and `/room/*` routes with Bearer token auth. Empty (default) = auth disabled.

```bash
export HIVEMIND_API_KEY=my-secret-key
curl -H "Authorization: Bearer my-secret-key" http://localhost:8080/v1/models
```

---

## Dashboard

Embedded SPA served at the root of `serve` (port 8080) or standalone via `web` (port 3000).

| Tab | Content |
|---|---|
| **Dashboard** | Resource monitor, peer cards, layer distribution map, distributed stats |
| **Chat** | Playground with real SSE streaming via the inference API |
| **Room** | Room info, invite code (copy button), leave/stop actions |

Shows a "No Active Room" screen with CLI instructions when no room exists.

---

## Configuration

Config file: `~/.hivemind/config.yaml`. All values overridable with `HIVEMIND_` env prefix.

```yaml
api:
  host: "127.0.0.1"        # bind address
  port: 8080
  rate_limit: 60            # req/s per IP
  max_body_bytes: 10485760  # 10 MB

signaling:
  url: "http://localhost:7777"
  port: 7777

mesh:
  wireguard_port: 51820
  grpc_port: 50052          # peer-to-peer gRPC

worker:
  grpc_port: 50051          # Go ↔ Python worker
  max_restarts: 3

resilience:
  max_retries: 3
  health_interval_s: 5
  circuit_max_fails: 3

log:
  level: "info"             # debug | info | warn | error
  format: "text"            # text | json
```

### Key Environment Variables

| Variable | Default | Description |
|---|---|---|
| `HIVEMIND_MOCK` | `false` | Mock mode (no GPU/Python needed) |
| `HIVEMIND_API_KEY` | *(empty)* | Bearer token auth (empty = disabled) |
| `HIVEMIND_SIGNALING_URL` | `http://localhost:7777` | Signaling server address |
| `HIVEMIND_PEER_ID` | *(auto)* | Override peer identity |
| `HIVEMIND_PEER_ENDPOINT` | *(auto-detect)* | Public WireGuard IP:port |
| `HIVEMIND_API_CORS_ORIGINS` | `localhost:5173,localhost:8080` | Allowed CORS origins |
| `HIVEMIND_WORKER_DIR` | `/app/worker` | Python worker path |
| `HIVEMIND_PYTHON_CMD` | `python3` | Python executable |

---

## Deployment

### Docker

```bash
docker build -t hivemind .

docker run -p 8080:8080 --gpus all hivemind                    # GPU
docker run -p 8080:8080 -e CUDA_VISIBLE_DEVICES="" hivemind    # CPU-only
docker run -p 8080:8080 -e HIVEMIND_MOCK=true hivemind         # Mock
```

### Signaling Server (Standalone)

```bash
./bin/hivemind signaling --port 7777          # as subcommand

# or as a separate lightweight image
docker build -t hivemind-signaling -f signaling-server/Dockerfile .
docker run -p 7777:7777 hivemind-signaling
```

### Production (1,000 Users)

See `deploy/production/`:

| File | Purpose |
|---|---|
| `docker-compose.yml` | nginx + 2x hivemind (GPU) + redis |
| `nginx.conf` | Load balancing, rate limiting, TLS, SSE |
| `config.yaml` | Tuned for 1k concurrent |
| `hivemind.service` | systemd with security hardening |
| `deploy.sh` | up / down / restart / logs / status / scale |

```bash
cd deploy/production && ./deploy.sh up
```

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                CLI  (cobra + lipgloss)                │
├──────────────────────────────────────────────────────┤
│             HTTP API  (chi · auth · rate-limit)      │
├──────────────┬──────────────────┬────────────────────┤
│  handlers/   │    services/     │      infra/        │
│  inference   │  real_room       │  signaling         │
│  room        │  real_inference  │  wireguard         │
│  catalog     │  resilience      │  peer_service      │
│  health      │  leader_election │  circuit_breaker   │
│  web (SPA)   │  diffusion       │  tensor_compress   │
│              │  metrics         │  worker_manager    │
├──────────────┴──────────────────┴────────────────────┤
│           gRPC  (Go ↔ Python Worker · Peer ↔ Peer)   │
├──────────────────────────────────────────────────────┤
│            WireGuard Mesh  (10.42.0.0/16)            │
└──────────────────────────────────────────────────────┘
```

**Layers:** `handlers → services → infra`. Interfaces define boundaries.

**Modes:** Real (default) uses signaling + WireGuard + gRPC + Python worker. Mock (`HIVEMIND_MOCK=true`) runs everything in-memory.

### Tech Stack

| | |
|---|---|
| **Backend** | Go 1.25, chi, slog, Viper |
| **Frontend** | React 19, TypeScript, Tailwind, Vite |
| **CLI** | cobra, lipgloss |
| **Networking** | WireGuard, gRPC + Protobuf |
| **AI Runtime** | Python, transformers, diffusers, torch |
| **Deploy** | Docker, nginx, systemd |

---

## Development

```bash
make build           # web + Go binary
make test            # Go + Python tests
make test-go         # Go tests with race detector
make test-python     # Python tests (pytest)
make proto           # regenerate protobuf
make lint            # go vet + golangci-lint + ruff
make coverage        # Go coverage report
```

### E2E Tests (Docker)

```bash
make test-e2e            # API tests — mock mode (32 assertions)
make test-e2e-scenario   # full user flow — mock (76 assertions)
make test-e2e-2users     # 2 containers — mock (90+ assertions)
make test-e2e-p2p        # signaling + 2 peers — real mode
make test-e2e-real       # GPU + CPU resource pooling
make test-e2e-down       # tear down all stacks
```

### Project Structure

```
cmd/hivemind/           Entry point, composition root
internal/
  api/                  HTTP server, middleware, auth
  catalog/              Model catalog (20 models, 5 types)
  cli/                  CLI commands
  config/               Viper config
  handlers/             HTTP handlers
  infra/                WireGuard, gRPC, signaling, circuit breaker
  logger/               Structured logging (slog)
  models/               Domain types
  services/             Business logic, orchestration
proto/                  Protobuf definitions
gen/                    Generated protobuf (Go)
signaling-server/       Standalone signaling binary + Dockerfile
web/                    React dashboard
worker/                 Python AI worker (transformers, diffusers)
tests/e2e/              E2E test scripts
deploy/production/      Docker, nginx, systemd
```

---

## Roadmap

- [x] Core types + CLI + Web dashboard
- [x] HTTP API + single-node inference
- [x] P2P mesh (WireGuard + signaling)
- [x] Tensor parallelism + zstd compression
- [x] Resilience (circuit breaker, leader election, health monitoring)
- [x] CI/CD + production deployment
- [x] **v1.0 — Full distributed P2P integration**
- [ ] Distributed token generation loop
- [ ] NAT traversal (STUN/TURN)
- [ ] Web UI for room creation/join
- [ ] Prometheus exporter
- [ ] Helm chart

---

## License

MIT — see [LICENSE](LICENSE).

## Contributing

Fork, branch, PR. Requires 1 approval + CI green.

## Support

[Issues](https://github.com/JohnPitter/hivemind/issues) · [Discussions](https://github.com/JohnPitter/hivemind/discussions)
