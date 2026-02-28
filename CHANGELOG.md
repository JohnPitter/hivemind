# Changelog

All notable changes to HiveMind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
