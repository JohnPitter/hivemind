# HiveMind — Design Document

> Distributed P2P system for cooperative AI model inference via tensor parallelism over the internet.

**Date:** 2026-02-27
**Status:** Draft
**Version:** 0.1.0

---

## 1. Vision

HiveMind allows multiple computers to form a cooperative cluster over the internet to run AI models too large for a single machine. A host creates a "room", participants join and share bare metal resources (GPU/CPU/RAM), and everyone gets access to an OpenAI-compatible API running the model distributed across all nodes via tensor parallelism.

**Core premise:** No single machine can run a 70B+ parameter model alone. But 3-4 machines with 8-12GB VRAM each can — if they split the model layers and coordinate.

### Key Decisions

| Aspect | Decision |
|--------|----------|
| **Cooperation model** | Host creates room, peers join via invite code, everyone contributes and everyone gets API access. No billing. |
| **AI models** | Both LLMs and image models. System-agnostic, host chooses what to load. |
| **Distribution** | Tensor parallelism over the internet (NOT load balancing). Models are split across machines. |
| **Tech stack** | Go (P2P orchestrator, API, networking) + Python (inference worker) |
| **P2P networking** | WireGuard mesh (Tailscale-style). Peers behave as if on the same LAN. |
| **Inference framework** | Hybrid: Petals-based for LLMs + custom pipeline parallelism for diffusion models |
| **Room discovery** | Centralized signaling server (matchmaking only). Zero trust in server. |
| **API** | OpenAI-compatible local server on each node (`localhost:8080`) |

---

## 2. Architecture

### 2.1 Three-Layer Architecture

```
+---------------------------------------------------+
|  API Layer (Go)           — handlers/              |
|  Receives requests, validates input, delegates     |
|  Never contains business logic                     |
+------------------------+---------------------------+
                         |
+------------------------v---------------------------+
|  Service Layer (Go)       — services/              |
|  All business logic lives here                     |
|  Room mgmt, peer mgmt, inference routing           |
+------------------------+---------------------------+
                         |
+------------------------v---------------------------+
|  Infrastructure Layer     — infra/                 |
|  WireGuard, gRPC, signaling client, storage        |
|  Adapters for external services                    |
+---------------------------------------------------+
```

### 2.2 Two Processes, One Machine

- **`hivemind`** (Go) — Main binary. Manages P2P networking, rooms, WireGuard, HTTP API. Spawns and manages the Python worker.
- **`hivemind-worker`** (Python) — Inference process. Receives commands from Go via local gRPC (`localhost:50051`). Loads model layers, runs inference, transfers tensors.

Go controls everything: network, rooms, API. Python only does inference — it is a worker that receives orders from Go.

### 2.3 Dependency Rules

- `handlers/` imports `services/` and `models/` (never `infra/`)
- `services/` imports `models/` and interfaces from `infra/` (dependency injection)
- `infra/` implements interfaces defined in `services/`
- `models/` has zero internal imports (pure domain)
- `proto/` generates code for both Go and Python (shared contract)

---

## 3. Master Principles (adapted for Go + Python)

### 3.1 Clean Architecture & Code

- **3-layer separation:** `handlers/` -> `services/` -> `infra/`
- Shared types via `.proto` (protobuf) — never duplicate interfaces
- No business logic in handlers — delegate to services
- DRY: extract utility functions to `pkg/` when pattern repeats 2+ times
- Single Responsibility: each file/function does ONE thing well
- Descriptive names: `AssignLayersToPeer()` > `Process()`, `IsPeerHealthy()` > `Check()`
- Short functions (< 50 lines); extract sub-functions if exceeded
- Barrel exports in each package for clean public API
- Constants in `internal/constants/` — never hardcode magic strings/numbers
- Organized imports: 1) stdlib, 2) external, 3) internal

### 3.2 Big O Performance

- O(1) preferred: Maps for peer lookup, layer assignment lookup
- Avoid O(n^2): never nested loops over peer/layer collections; use index/hash maps
- Pagination: `GET /room/status` with limit + offset for large rooms
- Batch tensor transfers when possible
- Tensor compression (zstd) to reduce network I/O ~40%
- gRPC streaming for large tensor transfers (avoid loading entire tensor in memory)

### 3.3 CVE Mitigation

- **Command Injection (CWE-78):** always `exec.Command` with args array, never concatenate
- **Path Traversal (CWE-22):** `filepath.Abs()` + validate prefix for model downloads
- **SSRF (CWE-918):** validate peer-provided URLs; whitelist allowed endpoints
- **Sensitive Data (CWE-200):** API never returns WireGuard private keys, room tokens in logs
- **Dependency vulnerabilities:** `govulncheck` + `pip audit` before each release
- **Rate limiting:** all API routes protected against brute force
- **Input validation:** every handler validates request body before passing to service

### 3.4 Resilience & Caching

- Retry with exponential backoff: tensor transfers 3 attempts (500ms, 1s, 2s)
- Timeout protection: gRPC health check 3s, tensor transfer 5-30s adaptive, model load 120s
- Circuit breaker: peer fails 3 consecutive health checks -> marked dead
- Graceful degradation: peer leaves -> layers redistributed, room continues
- Worker crash -> auto-restart (3 attempts with backoff 2s, 4s, 8s)
- Leader election: host dies -> lowest-IP peer takes over
- Model cache: downloaded layers cached at `~/.hivemind/cache/models/`

### 3.5 Testing Pyramid

- **Unit Tests (70%):** Go `*_test.go`, Python `pytest`. Services, utils, layer assignment algorithm
- **Integration Tests (20%):** gRPC client/server, WireGuard config generation, API endpoints
- **E2E Tests (10%):** Multi-node simulation (2+ processes on localhost)
- Coverage minimum: 60% overall, 80% for critical modules (inference routing, tensor transfer, layer assignment)
- Tests run in CI: `make test` on every PR
- Naming: `*_test.go` for unit, `*_integration_test.go` for integration

### 3.6 Security & Data Protection

- WireGuard keys never logged. Room tokens encrypted AES-256-GCM at rest.
- Zero secrets in plaintext. `~/.hivemind/keys/` with chmod 600.
- API bound to `127.0.0.1` only (never `0.0.0.0`).
- Tensor checksum SHA-256 on every transfer.
- Peer resource verification via benchmark on join.

### 3.7 Observability

- Structured logging with `slog` (Go), `structlog` (Python)
- Context tags: `[room:abc]`, `[peer:10.0.0.2]`, `[inference:req-123]`, `[mesh]`, `[worker]`
- Request logging: method, path, status_code, duration_ms
- Metrics: inter-peer latency, tokens/s, VRAM usage, tensor transfer size
- Every decision point has at least a `debug` log. No silent flows.

### 3.8 Phased Development

- Each feature is a numbered Phase (e.g., Phase 1)
- Complex phases divided into SubPhases with letters (e.g., 1A, 1B, 1C)
- Each SubPhase ends with: build passing, testable functionality, no regressions
- Small increments: each SubPhase = 1-3 hours of work, independently deployable
- Order of dependency: backend before frontend, types before everything

### 3.9 Changelog & Clean Build

- `CHANGELOG.md` with semantic versioning: Major (breaking), Minor (new feature), Patch (bugfix)
- `go vet` + `golangci-lint` + `pytest` must pass after every change
- Zero unused imports, zero unused variables
- Dead code removal: commented code must be deleted (git has history)

---

## 4. Project Structure

```
hivemind/
|-- cmd/
|   +-- hivemind/
|       +-- main.go                    # Entry point
|
|-- internal/
|   |-- handlers/                      # API Layer (zero logic)
|   |   |-- chat.go                    # POST /v1/chat/completions
|   |   |-- images.go                  # POST /v1/images/generations
|   |   |-- models.go                  # GET  /v1/models
|   |   |-- room.go                    # Room CRUD endpoints
|   |   +-- health.go                  # GET  /health, /status
|   |
|   |-- middleware/                     # Pipeline: logger -> rateLimit -> auth -> handler
|   |   |-- logger.go                  # Request logging with duration
|   |   |-- ratelimit.go               # Token bucket per IP
|   |   +-- auth.go                    # Room token validation
|   |
|   |-- services/                      # Business logic lives here
|   |   |-- room.go                    # Create/join/leave room, manage peers
|   |   |-- inference.go               # Route request -> correct worker, collect result
|   |   |-- mesh.go                    # Orchestrate WireGuard mesh
|   |   +-- worker.go                  # Manage Python process (start/stop/health)
|   |
|   |-- infra/                         # Infrastructure adapters
|   |   |-- wireguard/                 # WireGuard config generation, interface mgmt
|   |   |   |-- config.go
|   |   |   +-- interface.go
|   |   |-- signaling/                 # Client for signaling server
|   |   |   +-- client.go
|   |   |-- grpc/                      # gRPC client for Python worker
|   |   |   +-- worker_client.go
|   |   +-- process/                   # Process manager
|   |       +-- manager.go
|   |
|   |-- models/                        # Domain types (shared across layers)
|   |   |-- room.go                    # Room, Peer, RoomConfig
|   |   |-- inference.go               # InferenceRequest, InferenceResponse
|   |   +-- resource.go               # ResourceSpec (GPU, RAM, VRAM)
|   |
|   |-- config/                        # App configuration
|   |   +-- config.go                  # Viper-based, env vars + YAML
|   |
|   +-- logger/                        # Structured logging
|       +-- logger.go                  # slog wrapper with context tags
|
|-- proto/                             # Protobuf definitions (shared types)
|   |-- worker.proto                   # Go <-> Python worker communication
|   +-- peer.proto                     # Peer <-> Peer tensor transfer
|
|-- worker/                            # Python inference worker
|   |-- pyproject.toml                 # Dependencies (petals, torch, diffusers, grpcio)
|   |-- worker/
|   |   |-- __init__.py
|   |   |-- server.py                  # gRPC server
|   |   |-- inference/
|   |   |   |-- __init__.py
|   |   |   |-- llm.py                 # Petals-based LLM inference
|   |   |   +-- diffusion.py           # Pipeline parallelism for image models
|   |   |-- tensor/
|   |   |   |-- __init__.py
|   |   |   +-- transfer.py            # Tensor serialization/transfer
|   |   +-- resources/
|   |       |-- __init__.py
|   |       +-- detector.py            # Detect GPU, VRAM, RAM
|   +-- tests/
|       |-- test_llm.py
|       |-- test_diffusion.py
|       +-- test_transfer.py
|
|-- signaling-server/                  # Signaling server (separate deploy)
|   |-- main.go
|   |-- handlers/
|   |   +-- room.go
|   +-- Dockerfile
|
|-- docs/
|   +-- plans/
|
|-- CHANGELOG.md
|-- go.mod
|-- go.sum
|-- Makefile                           # build, test, lint, proto-gen
+-- .golangci.yml
```

---

## 5. Data Flows

### 5.1 Host Creates a Room

```
Host CLI                    Signaling Server
   |                              |
   |  1. POST /rooms              |
   |  {model: "llama-70b",        |
   |   maxPeers: 5,               |
   |   resources: {gpu: 8GB}}     |
   |---------------------------->|
   |                              |
   |  2. room_id + invite_code    |
   |  + host WireGuard pubkey     |
   |<----------------------------|
   |                              |
   |  3. Generate WireGuard keys  |
   |     Create interface wg0     |
   |     IP: 10.0.0.1/24          |
   |                              |
   |  4. Spawn Python worker      |
   |     gRPC localhost:50051     |
   |                              |
   |  5. Worker detects resources  |
   |     {gpu: "RTX 3060 12GB",   |
   |      vram_free: 10GB,        |
   |      ram_free: 24GB}         |
   |                              |
   |  6. API server starts        |
   |     localhost:8080            |
   |     Waiting for peers...     |
```

### 5.2 Peer Joins a Room

```
Peer CLI              Signaling Server           Host
   |                        |                      |
   |  1. POST /rooms/join   |                      |
   |  {invite_code: "abc",  |                      |
   |   resources: {8GB},    |                      |
   |   wg_pubkey: "xyz"}    |                      |
   |----------------------->|                      |
   |                        |  2. Notify host      |
   |                        |--------------------->|
   |                        |  3. Host approves    |
   |                        |<---------------------|
   |  4. Receive WG config  |                      |
   |<-----------------------|                      |
   |                                               |
   |  5. WireGuard handshake ---------------------->|
   |     Mesh connected!                           |
   |                                               |
   |  6. gRPC PeerSync: report resources --------->|
   |  7. Host assigns layers (e.g. 20-39) ------->|
   |  8. Worker loads assigned layers              |
```

### 5.3 Inference (Tensor Parallelism)

```
User App        Local API        Host (L0-19)      Peer (L20-39)
   |                |                 |                   |
   | POST /v1/chat  |                 |                   |
   | completions    |                 |                   |
   |--------------->|                 |                   |
   |                | 1. Tokenize     |                   |
   |                | 2. Route to host|                   |
   |                |---------------->|                   |
   |                |                 | 3. Forward L0-19  |
   |                |                 |                   |
   |                |                 | 4. Send tensor -->|
   |                |                 |                   |
   |                |                 | 5. Forward L20-39 |
   |                |                 |                   |
   |                |                 | 6. Return tensor  |
   |                |                 |<------------------|
   |                | 7. Decode       |                   |
   |                |<----------------|                   |
   | 8. Response    |                 |                   |
   |<---------------|                 |                   |
```

---

## 6. Resilience & Fault Tolerance

### 6.1 Peer Disconnects During Inference

1. WireGuard heartbeat fails (30s timeout)
2. gRPC health check fails (ping every 5s, 3 failures = dead)
3. Circuit breaker activates
4. In-flight request returns 503 (client retries with backoff)
5. Host recalculates layer assignment across remaining peers
6. Remaining peers download orphaned layers
7. Room continues with reduced total VRAM

### 6.2 Peer Joins Running Room (Hot-Join)

1. New peer connects via WireGuard mesh
2. Reports resources to host
3. Host recalculates layer assignment (now with more VRAM)
4. Host sends PAUSE to in-flight requests
5. Peers unload reassigned layers, new peer loads its layers
6. Host sends RESUME — inference resumes
7. Total downtime: ~10-30s (layer loading time)

### 6.3 Host Crashes (Leader Election)

1. Peers detect host is gone (WireGuard timeout)
2. Peer with lowest IP in mesh becomes new host
3. New host inherits resource registry + layer map (replicated via gRPC sync every 30s)
4. New host registers room with signaling server
5. Room continues without interruption

### 6.4 Unstable Network

- Tensor transfer with retry + backoff (3 attempts: 500ms, 1s, 2s)
- Adaptive timeout: measures average latency, sets timeout to 3x average (min 5s, max 30s)
- zstd tensor compression (~40% size reduction, no precision loss)
- Slow peer detection: latency > 5x room average triggers warning

### 6.5 Python Worker Crashes

1. Process manager detects PID exit or gRPC timeout
2. Auto-restart (max 3 attempts, backoff 2s/4s/8s)
3. Worker reloads layers from local cache
4. If 3 restarts fail, node removes itself from room

### 6.6 Timeouts and Limits

| Operation | Timeout | Retries | Backoff |
|-----------|---------|---------|---------|
| WireGuard handshake | 30s | - | - |
| gRPC health check | 3s | 3 failures = dead | - |
| Tensor transfer | adaptive 5-30s | 3 | 500ms, 1s, 2s |
| Model layer load | 120s | 1 | - |
| Worker restart | 10s | 3 | 2s, 4s, 8s |
| Signaling reconnect | 5s | infinite | 1s->30s cap |
| API request (user) | 60s | client-side | - |
| Leader election | 10s | - | - |
| Layer redistribution | 60s | 1 | - |

---

## 7. Security

### 7.1 Room Authentication

- Host generates Ed25519 keypair (room_key) on creation
- Signaling server returns `room_id` + `invite_code` (random 12 chars)
- `invite_code` shared out-of-band (WhatsApp, Discord, etc.)
- Peers join with invite_code + WireGuard pubkey
- Host approves/rejects (auto-approve or manual)

### 7.2 Transport Security (WireGuard)

- ChaCha20-Poly1305 encryption (256-bit)
- Unique keypair per peer
- Perfect Forward Secrecy
- All data (tensors, gRPC, health checks) travels inside encrypted tunnel
- No additional TLS needed

### 7.3 Local API Security

- Binds exclusively to `127.0.0.1` (never `0.0.0.0`)
- Room token required: `Authorization: Bearer <room_token>`
- Rate limiting: token bucket 60 req/min default
- Request size limit: 10MB

### 7.4 Malicious Peer Protection

| Attack | Mitigation |
|--------|------------|
| Corrupted tensor | SHA-256 checksum on every transfer. Mismatch = discard + retry |
| Free rider (consumes without contributing) | Resource benchmark on join. Minimum contribution enforced |
| Data snooping | Peers only see tensors for their layers, never full input/output |
| DDoS on mesh | Per-peer rate limiting on gRPC. >100 req/s = auto-disconnect |
| Compromised signaling server | Server only does matchmaking. Never sees WG private keys, room tokens, or tensors |
| Man-in-the-middle | WireGuard pubkeys exchanged via signaling. Fingerprint verifiable by host |

### 7.5 Code Security

```go
// CORRECT — args as array, never concatenate
exec.Command("wg", "set", "wg0", "peer", pubkey, "endpoint", endpoint)

// WRONG — command injection possible
exec.Command("bash", "-c", "wg set wg0 peer " + pubkey)  // NEVER

// CORRECT — path traversal prevention
func validateModelPath(p string) error {
    abs, _ := filepath.Abs(p)
    if !strings.HasPrefix(abs, allowedModelDir) {
        return ErrPathTraversal
    }
    return nil
}

// CORRECT — secrets never in logs
logger.Info("peer joined room", "peer_id", peer.ID, "room_id", room.ID)
// NEVER log: room_token, wg_private_key, invite_code
```

### 7.6 Data at Rest

```
~/.hivemind/
|-- config.yaml          # Non-sensitive configuration
|-- keys/
|   |-- wg_private.key   # chmod 600 — owner-only read
|   +-- wg_public.key
|-- rooms/
|   +-- <room_id>.json   # Room token encrypted AES-256-GCM
+-- cache/
    +-- models/           # Locally cached layers
```

---

## 8. Protobuf Contracts

### 8.1 Go <-> Python Worker (`proto/worker.proto`)

```protobuf
syntax = "proto3";
package hivemind.worker;

service WorkerService {
  rpc LoadModel(LoadModelRequest) returns (LoadModelResponse);
  rpc UnloadModel(UnloadModelRequest) returns (UnloadModelResponse);
  rpc GetStatus(StatusRequest) returns (StatusResponse);
  rpc ChatCompletion(ChatRequest) returns (ChatResponse);
  rpc ChatCompletionStream(ChatRequest) returns (stream ChatChunk);
  rpc ImageGeneration(ImageRequest) returns (ImageResponse);
  rpc ForwardPass(ForwardRequest) returns (ForwardResponse);
}

message LoadModelRequest {
  string model_id = 1;
  repeated int32 layers = 2;
  ModelType model_type = 3;
  enum ModelType { LLM = 0; DIFFUSION = 1; }
}

message LoadModelResponse {
  bool success = 1;
  string error = 2;
  ResourceUsage resources_used = 3;
}

message StatusResponse {
  WorkerState state = 1;
  string model_id = 2;
  repeated int32 loaded_layers = 3;
  ResourceUsage resources = 4;
  enum WorkerState { IDLE = 0; LOADING = 1; READY = 2; PROCESSING = 3; ERROR = 4; }
}

message ResourceUsage {
  int64 vram_total_mb = 1;
  int64 vram_used_mb = 2;
  int64 ram_total_mb = 3;
  int64 ram_used_mb = 4;
  string gpu_name = 5;
}

message ChatRequest {
  string request_id = 1;
  string model = 2;
  repeated ChatMessage messages = 3;
  float temperature = 4;
  int32 max_tokens = 5;
  bool stream = 6;
}

message ChatMessage {
  string role = 1;
  string content = 2;
}

message ChatResponse {
  string request_id = 1;
  string content = 2;
  UsageStats usage = 3;
}

message ChatChunk {
  string request_id = 1;
  string delta = 2;
  bool done = 3;
  UsageStats usage = 4;
}

message ImageRequest {
  string request_id = 1;
  string model = 2;
  string prompt = 3;
  int32 width = 4;
  int32 height = 5;
  int32 steps = 6;
  float guidance_scale = 7;
}

message ImageResponse {
  string request_id = 1;
  bytes image_data = 2;
  string format = 3;
}

message UsageStats {
  int32 prompt_tokens = 1;
  int32 completion_tokens = 2;
  int32 total_tokens = 3;
}

message ForwardRequest {
  string request_id = 1;
  bytes tensor_data = 2;
  TensorMeta meta = 3;
  bool compressed = 4;
  bytes checksum = 5;
}

message ForwardResponse {
  string request_id = 1;
  bytes tensor_data = 2;
  TensorMeta meta = 3;
  bool compressed = 4;
  bytes checksum = 5;
  float duration_ms = 6;
}

message TensorMeta {
  repeated int64 shape = 1;
  string dtype = 2;
  int32 from_layer = 3;
  int32 to_layer = 4;
}
```

### 8.2 Peer <-> Peer (`proto/peer.proto`)

```protobuf
syntax = "proto3";
package hivemind.peer;

service PeerService {
  rpc Handshake(HandshakeRequest) returns (HandshakeResponse);
  rpc SyncState(SyncRequest) returns (SyncResponse);
  rpc HealthCheck(Ping) returns (Pong);
  rpc ForwardTensor(ForwardRequest) returns (ForwardResponse);
  rpc ForwardTensorStream(stream ForwardRequest) returns (stream ForwardResponse);
}

message HandshakeRequest {
  string peer_id = 1;
  string room_token = 2;
  ResourceUsage resources = 3;
}

message HandshakeResponse {
  bool accepted = 1;
  string error = 2;
  RoomState room_state = 3;
}

message RoomState {
  string model_id = 1;
  int32 total_layers = 2;
  repeated PeerAssignment assignments = 3;
}

message PeerAssignment {
  string peer_id = 1;
  string peer_ip = 2;
  repeated int32 layers = 3;
  ResourceUsage resources = 4;
}

message SyncRequest {
  string peer_id = 1;
  uint64 state_version = 2;
}

message SyncResponse {
  RoomState room_state = 1;
  uint64 state_version = 2;
}

message Ping { int64 timestamp = 1; }
message Pong { int64 timestamp = 1; float latency_ms = 2; }
```

---

## 9. API Specification (OpenAI-compatible)

### 9.1 Chat Completions

```
POST /v1/chat/completions
Headers:
  Authorization: Bearer <room_token>
  Content-Type: application/json
Body:
  {
    "model": "meta-llama/Llama-3-70B",
    "messages": [
      {"role": "system", "content": "You are helpful."},
      {"role": "user", "content": "Hello!"}
    ],
    "temperature": 0.7,
    "max_tokens": 2048,
    "stream": false
  }
Response (non-stream):
  {
    "id": "hm-abc123",
    "object": "chat.completion",
    "created": 1709000000,
    "model": "meta-llama/Llama-3-70B",
    "choices": [{
      "index": 0,
      "message": {"role": "assistant", "content": "Hello!"},
      "finish_reason": "stop"
    }],
    "usage": {"prompt_tokens": 12, "completion_tokens": 3, "total_tokens": 15}
  }
Response (stream): SSE with data: {"delta": {"content": "tok"}}
```

### 9.2 Image Generation

```
POST /v1/images/generations
Body:
  {
    "model": "stabilityai/stable-diffusion-xl",
    "prompt": "a cat in space",
    "n": 1,
    "size": "1024x1024"
  }
Response:
  {
    "created": 1709000000,
    "data": [{"b64_json": "<base64 png>"}]
  }
```

### 9.3 Models

```
GET /v1/models
Response:
  {
    "data": [{
      "id": "meta-llama/Llama-3-70B",
      "object": "model",
      "owned_by": "hivemind-room"
    }]
  }
```

### 9.4 HiveMind-specific

```
POST   /room/create     # Create room (host only)
POST   /room/join       # Join room with invite code
DELETE /room/leave       # Leave room
GET    /room/status      # Peers, layers, resources, latency
GET    /health           # Local node health check
```

### 9.5 Middleware Pipeline

```
Request -> Logger -> RateLimit -> Auth -> Handler -> ErrorHandler -> Response
```

Error format: `{"error": {"message": "...", "code": "..."}}`

---

## 10. Implementation Roadmap

> **Priority: CLI & Web first.** Build the user-facing interfaces early with mocks/stubs,
> then wire up real backend services behind them incrementally.

### Phase 1: Scaffolding + Core Types

**Goal:** Project structure, build pipeline, and shared types ready.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **1A** | Project scaffolding: Go mod, Python pyproject, folder structure, Makefile, CI (.golangci.yml, GitHub Actions) | `go build ./...` passes, `pytest` passes |
| **1B** | Core domain types: `internal/models/` (Room, Peer, ResourceSpec, InferenceRequest), config with Viper, structured logger with slog | Types compile, config loads from YAML + env |
| **1C** | Mock services: `internal/services/` with interfaces + in-memory mock implementations (MockRoomService, MockInferenceService, MockWorkerService) | Mock services return realistic fake data |

### Phase 2: CLI Interativa ★

**Goal:** Full interactive CLI that works end-to-end with mock data. User can experience the complete workflow.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **2A** | CLI framework: cobra commands structure, lipgloss theme (colors, borders, formatting), global flags (`--config`, `--verbose`) | `hivemind --help` shows all commands with styled output |
| **2B** | `hivemind create`: interactive room creation wizard — select model (list from HuggingFace-like menu), set max peers, define resource contribution. Shows room summary + invite code with lipgloss box. Uses MockRoomService | `hivemind create` walks through wizard, shows invite code |
| **2C** | `hivemind join <code>`: join room flow — validates code, shows room info (model, peers, resources), confirms join, shows resource contribution form. Progress bar for "connecting" (mock) | `hivemind join abc123` shows room info and joins |
| **2D** | `hivemind status`: real-time room dashboard in terminal — peers table (name, IP, layers, VRAM, latency), model info, room health, auto-refresh every 2s with lipgloss table | `hivemind status` shows live-updating styled table |
| **2E** | `hivemind chat`: interactive chat in terminal — streaming token output, conversation history, `/quit` to exit. Like a mini ChatGPT in the terminal. Uses MockInferenceService | `hivemind chat` starts interactive session with streaming responses |
| **2F** | `hivemind leave` + `hivemind stop`: leave room gracefully, stop hosting. Confirmation prompts, cleanup feedback | Both commands work with confirmation |

### Phase 3: Web Dashboard ★

**Goal:** React SPA embedded in Go binary with real-time room monitoring and chat playground.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **3A** | Web project setup: Vite + React + TypeScript + Tailwind inside `web/`. Go embed for serving static files. HTTP API endpoints (`/api/room/status`, `/api/health`) returning mock JSON | `hivemind web` opens browser, dashboard loads |
| **3B** | Dashboard layout: dark theme sidebar (room info, peers list) + main content area. Header with room name, model, uptime. Responsive with Tailwind. Lucide React icons | Layout renders with mock data, responsive |
| **3C** | Peers panel: real-time peers table with status indicators (online/offline/syncing), VRAM usage bars, layer assignments, latency badges. Auto-refresh via polling or WebSocket | Peers panel shows all connected peers with live stats |
| **3D** | Resource monitor: VRAM/RAM usage charts (recharts), network throughput graph, tokens/s counter, inference queue depth. Cards with sparklines | Charts render with mock time-series data |
| **3E** | Chat playground: OpenAI-style chat interface — message input, streaming response display, model selector, temperature/max_tokens sliders, conversation history | Chat works with mock streaming responses |
| **3F** | Room management UI: create room form, join room form, QR code for invite code (qrcode.react), leave/stop buttons with confirmation dialogs | Full room lifecycle manageable from web UI |

### Phase 4: HTTP API + Single-Node Inference

**Goal:** Real HTTP API server and single-machine inference working. CLI and Web now use real data.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **4A** | HTTP API server: chi router, middleware pipeline (logger, rateLimit, auth), handlers for all endpoints, OpenAI-compatible response format | `curl /v1/chat/completions` returns properly formatted response |
| **4B** | Protobuf + gRPC setup: worker.proto, peer.proto, codegen for Go + Python | Generated types compile in both languages |
| **4C** | Python worker — local model: gRPC server, LoadModel, ChatCompletion, GetStatus, resource detection | Worker loads small model, generates text via gRPC |
| **4D** | Go process manager: spawn/kill Python worker, health checks, auto-restart with backoff | Go spawns worker, monitors health, restarts on crash |
| **4E** | Wire CLI + Web to real services: replace mock services with real implementations, SSE streaming for chat | CLI and Web use real inference, streaming works e2e |

### Phase 5: P2P + WireGuard Mesh

**Goal:** Peers behind NAT connect via WireGuard mesh.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **5A** | WireGuard integration: keypair gen, wg0.conf gen, interface up/down programmatically | 2 nodes connected via WireGuard tunnel |
| **5B** | Signaling server: Go HTTP + WebSocket, room create/join, WG pubkey exchange, Dockerfile | 2 nodes discover via signaling, connect WG automatically |
| **5C** | Room lifecycle: create/join/leave/close with real WireGuard mesh, room token auth | Full room lifecycle works over internet |

### Phase 6: Tensor Parallelism

**Goal:** Models split across multiple machines, inference works distributed.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **6A** | PeerService gRPC: handshake, resource registry, layer assignment algorithm | 2 nodes exchange state, layers assigned by VRAM |
| **6B** | Tensor transfer: ForwardPass, safetensors serialization, zstd compression, SHA-256 checksum | Tensors transfer between nodes correctly |
| **6C** | Distributed inference: full pipeline — tokenize → split forward pass → collect → decode. SSE streaming | 70B model split across 2+ machines, generates text |
| **6D** | CLI + Web updates: status shows real peer layers, latency, tensor transfer metrics | Dashboard shows live distributed inference stats |

### Phase 7: Resilience + Image Models

**Goal:** Production-ready system with fault tolerance and image model support.

| SubPhase | Description | Verification |
|----------|-------------|--------------|
| **7A** | Fault tolerance: peer disconnect detection, layer redistribution, hot-join, worker restart | Kill peer mid-inference, room recovers |
| **7B** | Leader election: host detection, election by lowest IP, state replication every 30s | Kill host, new leader takes over seamlessly |
| **7C** | Diffusion pipeline: TextEncoder → UNet → VAE stages, stage assignment, POST /v1/images/generations | SDXL split across 2 nodes, generates image |
| **7D** | Observability: structured logging, context tags, metrics endpoint, GET /room/status with full stats | Logs traceable e2e, metrics visible on dashboard |

### Estimated Effort

| Phase | SubPhases | Estimated Effort |
|-------|-----------|-----------------|
| **1: Scaffolding** | 3 (1A-1C) | 1 week |
| **2: CLI** ★ | 6 (2A-2F) | 2 weeks |
| **3: Web Dashboard** ★ | 6 (3A-3F) | 2-3 weeks |
| **4: API + Inference** | 5 (4A-4E) | 2-3 weeks |
| **5: P2P + WireGuard** | 3 (5A-5C) | 2 weeks |
| **6: Tensor Parallelism** | 4 (6A-6D) | 2-3 weeks |
| **7: Resilience + Images** | 4 (7A-7D) | 2 weeks |

**Interactive MVP (Phase 1+2+3): ~5-6 weeks** — Full CLI + Web Dashboard with mock inference.
**Functional MVP (Phase 1-5): ~9-11 weeks** — Real distributed inference over internet with UI.

---

## 11. Technology Reference

| Component | Technology | Why |
|-----------|-----------|-----|
| P2P orchestrator | Go | Performance, single binary, excellent networking stdlib |
| Inference worker | Python | PyTorch/Petals/Diffusers ecosystem, no alternative |
| Inter-process comm | gRPC + Protobuf | Typed contracts, streaming, code generation for both languages |
| P2P networking | WireGuard | Best NAT traversal, encrypted tunnel, LAN-like behavior |
| Signaling | Go HTTP + WebSocket | Lightweight matchmaking server |
| LLM inference | Petals | Battle-tested tensor parallelism for LLMs over internet |
| Image inference | Custom + Diffusers | Pipeline parallelism leveraging natural diffusion stages |
| Tensor compression | zstd | Fast compression, ~40% reduction, no precision loss |
| Tensor serialization | safetensors | Safe, fast, standard format |
| Configuration | Viper | YAML + env vars + defaults |
| Logging | slog (Go), structlog (Python) | Structured, context-tagged |
| CLI framework | cobra + lipgloss | Standard Go CLI tools |
| HTTP router | chi | Lightweight, middleware-friendly |
| Testing | Go testing + pytest | Standard for each ecosystem |
| CI/CD | GitHub Actions | Automated build + test + lint |
