# Napkin — HiveMind

## Corrections
| Date | Source | What Went Wrong | What To Do Instead |
|------|--------|----------------|-------------------|
| 2026-02-28 | self | Used HTTP request context for `wm.Start(ctx)` — worker killed when request completed | Long-lived processes (Python worker) MUST use `context.Background()`, never request-scoped contexts |
| 2026-02-28 | self | Used `python3 -m worker.server` but server.py has no `if __name__` block | Use `python3 -m worker` to hit `__main__.py` which explicitly calls `serve()` |
| 2026-02-28 | self | Set worker startup timeout to 30s — torch/transformers import takes >30s | Use 120s timeout for Python workers that import heavy ML libraries |
| 2026-02-28 | self | MockRoomService.Join() hardcoded model ID, ignoring env var | Read `HIVEMIND_MODEL_ID` env var in Join() for real inference mode |
| 2026-02-28 | self | Asserted exact peer count but each Docker container has separate MockRoomService | Each container is isolated — assert field existence, not cross-container state |

## User Preferences
- Communicates in Portuguese (pt-BR): "pode seguir", "funcionou?", "deu certo?"
- Prefers concise progress updates
- Wants to be told when tests pass/fail with numbers

## Patterns That Work
- Debug prints with `flush=True` in Python for Docker log visibility
- `PYTHONUNBUFFERED=1` env var for Python subprocess stdout in Go
- `context.Background()` for long-lived child processes spawned from request handlers
- Using `__main__.py` as entry point for `python -m <package>` pattern
- Exponential backoff with 500ms start + 5s cap for gRPC readiness checks
- Shared HF cache volume in Docker Compose to avoid re-downloading models

## Patterns That Don't Work
- Passing HTTP request context to `exec.CommandContext` for workers — context cancels on response
- `python -m package.module` when module has no `if __name__` guard — functions defined but never called
- 30s timeout for torch/transformers imports in Docker — too short, need 120s
- Asserting cross-container state in separate MockRoomService instances

## Domain Notes
- HiveMind: Go orchestrator + Python worker (gRPC bridge on :50051)
- WorkerManager spawns Python, monitors health, auto-restarts with backoff
- RealInferenceService wraps gRPC calls to Python worker
- MockRoomService simulates P2P rooms per-container (no real networking)
- TinyLlama-1.1B-Chat: 22 layers, ~2.2GB float16, good for testing
- `detector.py` has `total_mem` bug — should be `total_memory()` for newer torch (handled by fallback)
- Docker GPU: `nvidia/cuda:12.4.0-runtime-ubuntu22.04`, CPU-only via `CUDA_VISIBLE_DEVICES=""`
