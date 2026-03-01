# Changelog

All notable changes to HiveMind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-02-28

### Phase 9: Distributed P2P Inference — Full Integration

#### Phase 9A: Config + Bug Fixes

##### Added
- `internal/config/config.go` — `SignalingConfig`, `MeshConfig`, `PeerConfig`, `ResilienceConfig` structs with sane defaults
- `internal/api/auth.go` — `APIKeyAuth` middleware (Bearer token via `HIVEMIND_API_KEY`) and `MaxBodyMiddleware`

##### Changed
- `internal/api/middleware.go` — Fixed rate limiter IP parsing: `net.SplitHostPort(r.RemoteAddr)` + `X-Forwarded-For` header support
- `internal/api/middleware.go` — `CORSMiddleware` now accepts `allowedOrigins []string` instead of wildcard `*`
- `internal/handlers/web.go` — `HandleHealthJSON` reads real data from `roomSvc.CurrentRoom()` instead of hardcoded values
- `internal/api/server.go` — Accepts `*config.Config`, reads `HIVEMIND_API_KEY` env, applies auth + body limit middleware to `/v1` and `/room` routes
- `cmd/hivemind/main.go` — Version bumped to `1.0.0`

##### Security
- Rate limiter correctly extracts client IP behind reverse proxies
- CORS restricted to explicit origin whitelist (default: localhost:5173, localhost:8080)
- API key auth protects inference and room management endpoints
- Request body size limited via `http.MaxBytesReader`

#### Phase 9B: RealRoomService

##### Added
- `internal/services/real_room.go` — Full room orchestrator: signaling → WireGuard → PeerRegistry → PeerGRPCServer with Create/Join/Leave/Stop/Status flows
- `internal/infra/peer_id.go` — `GetOrCreatePeerID()` persists peer identity in `~/.hivemind/peer_id`

#### Phase 9C: main.go Wiring + Signaling Subcommand

##### Added
- `internal/cli/signaling.go` — `hivemind signaling --port 7777` subcommand

##### Changed
- `cmd/hivemind/main.go` — Composition root rewritten: conditionally creates real services (WireGuard, Signaling, PeerRegistry, RealRoomService, RealInferenceService) when `HIVEMIND_MOCK != true`; mock mode preserved for development/testing

#### Phase 9D: Docker Compose P2P Test

##### Added
- `docker-compose.p2p.yml` — 3-container stack (signaling + alice + bob) for P2P wiring validation
- `tests/e2e/p2p_wiring.sh` — P2P E2E test: signaling health → room creation → peer join → peer visibility → leave → cleanup

#### Phase 9E: Python forward_pass + tensor/transfer.py

##### Changed
- `worker/worker/tensor/transfer.py` — Implemented real tensor serialization: `serialize_tensor()` / `deserialize_tensor()` with numpy wire format (dtype + ndim + shape + raw bytes)
- `worker/worker/inference/llm.py` — `forward_pass()` now runs real transformer layers (detects Llama/Mistral, GPT-NeoX, GPT-2/GPT-J architectures); identity fallback when model is None

##### Added
- `worker/tests/test_tensor.py` — 5 round-trip serialization tests
- `worker/tests/test_forward.py` — 2 forward pass tests with mock model

#### Phase 9F: DistributedInferenceService Fix

##### Changed
- `internal/services/real_inference.go` — Fixed layer loading bug: `ensureModelLoaded()` now loads only the local peer's assigned layers instead of all peers' layers; added `SetLocalPeerID()` method

##### Removed
- `internal/services/grpc_inference.go` — Deleted redundant file (functionality consolidated into `real_inference.go`)

#### Phase 9G: Consolidation

##### Added
- `internal/services/helpers.go` — Extracted shared functions: `generateID()`, `assignLayers()`, `makeRange()`, `parseSize()`

##### Changed
- `internal/services/mock_room.go` — Removed duplicate function definitions, now imports from helpers.go

#### Phase 9H: Web Dashboard API

##### Added
- `web/src/lib/api.ts` — Real API client: `fetchRoomStatus()`, `fetchHealth()`, `leaveRoom()`, `stopRoom()`, `chatCompletionStream()` (SSE async generator)
- `web/src/hooks/useRoomStatus.ts` — `useRoomStatus()` hook polling `/api/room/status` every 5s

##### Changed
- `web/src/App.tsx` — Replaced `mockRoomStatus` with `useRoomStatus()` hook; added `LoadingScreen` and `NoRoomScreen` states; `RoomInfo` buttons call real leave/stop API
- `web/src/components/ChatPlayground.tsx` — Replaced `mockStreamResponse` with real `chatCompletionStream()` async generator; added error state display

#### Phase 9I: API Key Auth

##### Added
- `internal/api/auth.go` — `APIKeyAuth(key)` middleware: validates `Authorization: Bearer <key>`, returns 401/403 JSON errors; disabled when `HIVEMIND_API_KEY` env is empty

#### Phase 9J: Rate Limiter Hardening

##### Changed
- `internal/api/server.go` — Rate limit configurable via `cfg.API.RateLimit`; request body size limited via `cfg.API.MaxBodyBytes`

#### Phase 9K: Signaling Standalone

##### Added
- `signaling-server/main.go` — Standalone signaling server binary (~20 lines) for separate deployment
- `signaling-server/Dockerfile` — Lightweight Alpine-based image for signaling-only deployments

#### Phase 9L: E2E P2P Tests

##### Changed
- `docker-compose.test.yml` — Added `p2p` profile with signaling, alice-p2p, bob-p2p, and p2p-wiring-tests containers
- `Makefile` — Added `test-e2e-p2p` target; updated `test-e2e-down` to clean up P2P stacks

## [0.8.0] - 2026-02-27

### Production Deployment + CI/CD

#### Added
- `.github/workflows/ci.yml` — GitHub Actions CI: build, test (with race detector + coverage), lint (go vet) — status checks required for master branch protection
- `Dockerfile` — Multi-stage production build: Node 20 web build → Go static binary → scratch image (~25MB), version injection from git tags
- `.dockerignore` — Excludes .git, node_modules, deploy, docs from Docker context
- `deploy/production/config.yaml` — Production configuration for 1,000 concurrent users: 500 req/s rate limit, 50MB tensor body limit, /16 mesh subnet (65k peers), circuit breaker tuning, JSON logging, zstd compression
- `deploy/production/docker-compose.yml` — Full production stack: nginx (load balancer) → 2× hivemind (GPU-enabled) → redis (session/room state), health checks, resource limits, NVIDIA runtime
- `deploy/production/nginx.conf` — Reverse proxy: least-conn upstream balancing, SSE streaming support for chat, tiered rate limiting (inference 10r/s, API 50r/s, general 100r/s), TLS 1.2/1.3, security headers, gzip, JSON access logs, /metrics restricted to internal networks
- `deploy/production/hivemind.service` — systemd service unit: security hardening (NoNewPrivileges, ProtectSystem, PrivateTmp), 65535 file descriptor limit, 8GB memory cap, auto-restart on failure
- `deploy/production/deploy.sh` — Deploy script: up/down/restart/logs/status/scale commands, prerequisite checks, color output, health check verification
- `deploy/production/env.example` — Environment variable template for production secrets

#### Changed
- Branch protection enabled on `master`: requires PR with 1 approval, dismiss stale reviews, status checks (build + test) must pass

## [0.7.0] - 2026-02-27

### Phase 7: Resilience + Image Models

#### Added
- `internal/infra/circuit_breaker.go` — Per-peer circuit breaker: closed/open/half-open states, configurable max failures and reset timeout, state change callbacks
- `internal/infra/circuit_breaker_test.go` — 7 tests: initial state, opens after max failures, success resets, half-open after timeout, half-open to closed, state change callback, manual reset
- `internal/infra/health_monitor.go` — Peer health monitor: periodic gRPC health checks (5s interval), consecutive failure tracking, auto-mark dead after 3 fails, EMA latency calculation, peer recovery detection
- `internal/services/resilience.go` — Resilience service: retry with exponential backoff (generic `WithRetry[T]`), adaptive timeout based on latency, slow peer detection, layer redistribution on peer death, peer recovery handling
- `internal/services/resilience_test.go` — 6 tests: retry success first attempt, retry after failures, all retries fail, context canceled, adaptive timeout, slow peer detection, exponential backoff
- `internal/services/leader_election.go` — Leader election: lowest-IP strategy, host death detection, election among alive peers, `onBecomeLeader` callback, IP comparison
- `internal/services/leader_election_test.go` — 5 tests: lowest IP wins, local becomes leader with callback, not local leader, empty peers, single peer
- `internal/services/diffusion_pipeline.go` — Diffusion pipeline parallelism: 3-stage pipeline (TextEncoder → UNet → VAE Decoder), VRAM-based stage assignment (UNet to strongest peer), distributed image generation
- `internal/services/diffusion_pipeline_test.go` — 4 tests: single peer all stages, two peers UNet to strongest, three peers distributed, empty peers
- `internal/services/metrics.go` — Metrics collector: atomic counters for inference/tensor/peer metrics, latency percentiles (P50/P95/P99), request duration histogram, point-in-time snapshots

#### Changed
- `internal/logger/logger.go` — Enhanced with context-aware logging (`WithContext`, `FromContext`, `CtxInfo`), JSON output mode (`InitJSON`), semantic loggers (`Room()`, `Peer()`, `Inference()`, `Mesh()`, `Worker()`)
- `internal/handlers/health.go` — Added `GET /metrics` endpoint returning `MetricsSnapshot` with latency percentiles, transfer counts, error rates
- `internal/api/server.go` — Added `/metrics` route
- `internal/infra/peer_service.go` — Added `Client()` accessor method to `PeerNode`

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
