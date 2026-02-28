# Changelog

All notable changes to HiveMind will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
