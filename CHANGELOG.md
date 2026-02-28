# Changelog

All notable changes to HiveMind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.0] - 2026-02-27

### Phase 6: Tensor Parallelism

#### Added
- `internal/infra/peer_service.go` — PeerRegistry: gRPC connection management per peer, handshake, latency measurement, forward tensor routing
- `internal/infra/peer_grpc_server.go` — Peer gRPC server: implements PeerService (Handshake, SyncState, HealthCheck, ForwardTensor, ForwardTensorStream) with tensor compression
- `internal/infra/tensor_compress.go` — TensorCompressor: zstd compression/decompression for tensor transfers (~40% size reduction), stream support, automatic beneficial compression detection
- `internal/infra/tensor_compress_test.go` — 6 tests: round-trip, empty data, random data, compress-if-beneficial, should-compress threshold, stream compression
- `internal/services/distributed_inference.go` — Distributed inference coordinator: VRAM-proportional layer assignment, forward pass chaining across peers, SHA-256 checksums, zstd compression, transfer metrics tracking
- `internal/services/distributed_inference_test.go` — 4 tests: VRAM-proportional assignment, equal fallback (zero VRAM), single peer, empty inputs
- `web/src/components/DistributedPanel.tsx` — Web dashboard panel: tensor transfer count, compression ratio, forward pass latency, pipeline visualization with animated progress bars

#### Changed
- `internal/models/room.go` — Added `DistributedStats` type and optional `Distributed` field to `RoomStatus` for distributed inference metrics
- `internal/cli/status.go` — Added distributed inference stats section: transfer count, bytes transferred, compression ratio, forward pass avg, latency, mode indicator
- `web/src/types/index.ts` — Added `DistributedStats` interface to TypeScript types
- `web/src/lib/mock-data.ts` — Added distributed stats mock data
- `web/src/App.tsx` — Integrated DistributedPanel into dashboard view
- `go.mod` — Added `github.com/klauspost/compress` for zstd tensor compression

## [0.5.0] - 2026-02-27

### Phase 5: P2P + WireGuard Mesh

#### Added
- `internal/infra/wireguard.go` — WireGuard manager: Curve25519 keypair generation, mesh IP allocation, peer add/remove, config file generation
- `internal/infra/signaling.go` — Signaling server: in-memory room registry, create/join/leave/peers/health HTTP endpoints, WG key exchange
- `internal/infra/signaling_client.go` — Signaling client: HTTP client for room creation, joining, leaving, and peer discovery
- `internal/infra/wireguard_test.go` — 5 tests: keypair generation, uniqueness, initialization, peer management, config writing
- `internal/infra/signaling_test.go` — 3 tests: create/join request marshaling, join response structure

## [0.4.0] - 2026-02-27

### Phase 4: HTTP API + Single-Node Inference

#### Added
- `internal/api/server.go` — Chi-based HTTP server with middleware pipeline and OpenAI-compatible routes
- `internal/api/middleware.go` — Request logger, token-bucket rate limiter, CORS, panic recovery middleware
- `internal/handlers/inference.go` — POST /v1/chat/completions (streaming + non-streaming), POST /v1/images/generations, GET /v1/models
- `internal/handlers/room.go` — POST /room/create, POST /room/join, DELETE /room/leave, GET /room/status
- `internal/handlers/health.go` — GET /health endpoint with worker and peer status
- `internal/handlers/common.go` — Shared JSON response helpers, error mapping (domain errors -> HTTP status), SPA handler
- `internal/handlers/inference_test.go` — 5 tests: non-streaming chat, empty messages, invalid JSON, streaming SSE, list models
- `internal/handlers/room_test.go` — 5 tests: create room, missing model, status not in room, status in room, leave not in room
- `proto/worker.proto` — gRPC contract: LoadModel, GetStatus, ChatCompletion, ChatCompletionStream, ImageGeneration, ForwardPass
- `proto/peer.proto` — gRPC contract: Handshake, SyncState, HealthCheck, ForwardTensor
- `gen/workerpb/` — Generated Go protobuf + gRPC code for worker service
- `gen/peerpb/` — Generated Go protobuf + gRPC code for peer service
- `worker/worker/gen/` — Generated Python protobuf + gRPC code
- `internal/infra/worker_manager.go` — Python worker process manager: spawn, health monitoring, auto-restart with exponential backoff
- `internal/services/grpc_inference.go` — Real InferenceService implementation delegating to Python worker via gRPC
- `internal/services/worker_service.go` — Real WorkerService wrapping WorkerManager
- `worker/worker/server.py` — Full gRPC server implementation with WorkerServicer
- `worker/worker/inference/llm.py` — LLM engine with transformers support + mock fallback, streaming generation
- `worker/worker/inference/diffusion.py` — Diffusion engine with diffusers support + mock PNG generation

#### Changed
- `internal/handlers/web.go` — Exported handler methods for chi integration, moved shared helpers to common.go
- `Makefile` — Updated proto target for module-relative output paths

## [0.3.0] - 2026-02-27

### Phase 3: Web Dashboard

#### Added
- `web/` — Vite + React + TypeScript + Tailwind dashboard project
- `web/src/App.tsx` — Main layout with sidebar, header, tab routing (dashboard/chat/room)
- `web/src/components/Sidebar.tsx` — Dark sidebar with room status and navigation
- `web/src/components/Header.tsx` — Stats bar with peers, speed, VRAM, uptime
- `web/src/components/PeersPanel.tsx` — Peer cards with GPU info, VRAM bars, latency
- `web/src/components/ResourceMonitor.tsx` — 4 resource stat cards with progress bars
- `web/src/components/LayerMap.tsx` — Visual layer distribution map
- `web/src/components/ChatPlayground.tsx` — Chat interface with streaming mock responses
- `web/embed.go` — Go embed directive for dist/ directory
- `internal/cli/web.go` — `hivemind web` command serving embedded SPA on port 3000
- `internal/handlers/web.go` — HTTP handler for dashboard API and SPA fallback

#### Changed
- `cmd/hivemind/main.go` — Integrated web embed package and web command

## [0.2.0] - 2026-02-27

### Phase 2: CLI Interativa

#### Added
- `internal/cli/theme.go` — Lipgloss theme with amber/honey color palette, ASCII logo, status indicators
- `internal/cli/commands.go` — Command registration hub
- `internal/cli/create.go` — `hivemind create` with interactive model selection menu (6 popular models + custom), lipgloss styled output with invite code box
- `internal/cli/join.go` — `hivemind join <code>` with animated connection progress steps
- `internal/cli/status.go` — `hivemind status` with VRAM usage bar, peers table (GPU, layers, latency), visual layer distribution map
- `internal/cli/chat.go` — `hivemind chat` with streaming token output, conversation history, /quit /clear /help commands
- `internal/cli/leave.go` — `hivemind leave` and `hivemind stop` with confirmation prompts

#### Changed
- `cmd/hivemind/main.go` — Integrated mock services and registered all CLI commands
- Added charm libraries: lipgloss, bubbles, bubbletea

### Phase 1B: Core Domain Types

#### Added
- `internal/models/room.go` — Room, Peer, RoomConfig, RoomStatus types with state enums
- `internal/models/inference.go` — OpenAI-compatible ChatRequest/Response, ImageRequest/Response, streaming chunks
- `internal/models/resource.go` — ResourceSpec with VRAM/RAM tracking and usable VRAM calculation
- `internal/models/errors.go` — Sentinel errors for room, peer, inference, network operations

### Phase 1C: Mock Services

#### Added
- `internal/services/interfaces.go` — RoomService, InferenceService, WorkerService interfaces
- `internal/services/mock_room.go` — Full mock with create/join/leave/status, VRAM-proportional layer assignment algorithm
- `internal/services/mock_inference.go` — Mock chat completion with word-by-word streaming, mock image generation
- `internal/services/mock_worker.go` — Mock worker with GPU resource reporting
- `internal/services/mock_room_test.go` — 6 unit tests covering room lifecycle and layer assignment

## [0.1.0] - 2026-02-27

### Phase 1A: Project Scaffolding

#### Added
- Go module initialization (`go.mod`)
- Project folder structure following clean architecture
- `cmd/hivemind/main.go` — CLI entry point with cobra
- `internal/config/config.go` — Viper-based configuration (YAML + env vars)
- `internal/logger/logger.go` — Structured logging wrapper with slog
- `Makefile` — Build, test, lint, proto-gen, clean targets
- `.golangci.yml` — Linter configuration
- `.gitignore` — Go, Python, IDE, OS exclusions
- Python worker skeleton (`worker/pyproject.toml`)
- Design document at `docs/plans/2026-02-27-hivemind-design.md`
