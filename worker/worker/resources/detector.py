"""Detect available system resources (GPU, VRAM, RAM)."""

from __future__ import annotations

import platform
from dataclasses import dataclass

import structlog

logger = structlog.get_logger("worker.resources.detector")


@dataclass
class ResourceInfo:
    """Detected system resources."""

    gpu_name: str
    vram_total_mb: int
    vram_free_mb: int
    ram_total_mb: int
    ram_free_mb: int
    cuda_available: bool
    platform: str


def detect() -> ResourceInfo:
    """Detect available GPU, VRAM, and RAM."""
    logger.info("detecting system resources")

    gpu_name = "CPU only"
    vram_total = 0
    vram_free = 0
    cuda_available = False

    try:
        import torch

        if torch.cuda.is_available():
            cuda_available = True
            gpu_name = torch.cuda.get_device_name(0)
            vram_total = torch.cuda.get_device_properties(0).total_mem // (1024 * 1024)
            vram_free = vram_total - (torch.cuda.memory_allocated(0) // (1024 * 1024))
    except ImportError:
        logger.warn("torch not installed, GPU detection skipped")

    import psutil

    ram = psutil.virtual_memory()
    ram_total = ram.total // (1024 * 1024)
    ram_free = ram.available // (1024 * 1024)

    info = ResourceInfo(
        gpu_name=gpu_name,
        vram_total_mb=vram_total,
        vram_free_mb=vram_free,
        ram_total_mb=ram_total,
        ram_free_mb=ram_free,
        cuda_available=cuda_available,
        platform=platform.system(),
    )

    logger.info(
        "resources detected",
        gpu=info.gpu_name,
        vram_total_mb=info.vram_total_mb,
        vram_free_mb=info.vram_free_mb,
        ram_total_mb=info.ram_total_mb,
        ram_free_mb=info.ram_free_mb,
    )

    return info
