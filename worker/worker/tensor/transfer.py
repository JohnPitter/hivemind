"""Tensor serialization and transfer via gRPC."""

import structlog

logger = structlog.get_logger("worker.tensor.transfer")

# TODO: Phase 6B — implement tensor serialization with safetensors + zstd compression
