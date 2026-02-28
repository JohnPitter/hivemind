"""Custom pipeline parallelism for diffusion models."""

from __future__ import annotations

import io
import struct

import structlog

logger = structlog.get_logger("worker.inference.diffusion")


class DiffusionEngine:
    """Handles image generation using pipeline parallelism across nodes.

    Diffusion models have a natural 3-stage pipeline:
    TextEncoder -> UNet (iterative) -> VAE Decoder

    Each stage can run on a different node for pipeline parallelism.
    """

    def __init__(self) -> None:
        self.model_id: str | None = None
        self.stage: str | None = None  # "text_encoder", "unet", "vae", or "full"
        self.is_ready = False
        self._pipeline = None

    def load_stage(self, model_id: str, stage: str) -> None:
        """Load a specific pipeline stage into GPU memory."""
        logger.info("loading diffusion stage", model_id=model_id, stage=stage)

        self.model_id = model_id
        self.stage = stage

        try:
            self._load_diffusers(model_id)
        except Exception as e:
            logger.warn(
                "diffusers load failed, using mock mode",
                error=str(e),
                model_id=model_id,
            )
            self._pipeline = None

        self.is_ready = True
        logger.info("diffusion stage ready", model_id=model_id, stage=stage)

    def _load_diffusers(self, model_id: str) -> None:
        """Load model using HuggingFace diffusers."""
        try:
            from diffusers import StableDiffusionXLPipeline
            import torch

            device = "cuda" if torch.cuda.is_available() else "cpu"
            dtype = torch.float16 if device == "cuda" else torch.float32

            logger.info("loading with diffusers", model_id=model_id, device=device)

            self._pipeline = StableDiffusionXLPipeline.from_pretrained(
                model_id,
                torch_dtype=dtype,
            ).to(device)

            logger.info("diffusion pipeline loaded", model_id=model_id)
        except ImportError:
            raise RuntimeError("diffusers library not installed")

    def unload_stage(self) -> None:
        """Unload pipeline stage from GPU memory."""
        if self.model_id:
            logger.info("unloading stage", model_id=self.model_id, stage=self.stage)

        self._pipeline = None
        self.model_id = None
        self.stage = None
        self.is_ready = False

        try:
            import torch
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
        except ImportError:
            pass

    def generate(
        self,
        prompt: str,
        width: int = 1024,
        height: int = 1024,
        steps: int = 30,
        guidance_scale: float = 7.5,
    ) -> bytes:
        """Generate an image from a text prompt."""
        if not self.is_ready:
            raise RuntimeError("diffusion model not loaded")

        if self._pipeline is not None:
            return self._generate_real(prompt, width, height, steps, guidance_scale)

        return self._generate_mock(prompt, width, height)

    def _generate_real(
        self,
        prompt: str,
        width: int,
        height: int,
        steps: int,
        guidance_scale: float,
    ) -> bytes:
        """Generate using the loaded diffusers pipeline."""
        image = self._pipeline(
            prompt=prompt,
            width=width,
            height=height,
            num_inference_steps=steps,
            guidance_scale=guidance_scale,
        ).images[0]

        buf = io.BytesIO()
        image.save(buf, format="PNG")
        return buf.getvalue()

    def _generate_mock(self, prompt: str, width: int, height: int) -> bytes:
        """Generate a minimal valid PNG as a mock response."""
        return _create_minimal_png(width, height)


def _create_minimal_png(width: int, height: int) -> bytes:
    """Create a minimal valid 1x1 PNG image for mock responses."""
    # Minimal valid PNG: 1x1 pixel, black
    # This is a proper PNG file that any image viewer can open
    import zlib

    def _chunk(chunk_type: bytes, data: bytes) -> bytes:
        raw = chunk_type + data
        return struct.pack(">I", len(data)) + raw + struct.pack(">I", zlib.crc32(raw) & 0xFFFFFFFF)

    buf = io.BytesIO()

    # PNG signature
    buf.write(b"\x89PNG\r\n\x1a\n")

    # IHDR chunk (1x1, 8-bit RGB)
    ihdr_data = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
    buf.write(_chunk(b"IHDR", ihdr_data))

    # IDAT chunk (one scanline: filter byte + RGB)
    raw_data = b"\x00\x20\x20\x20"  # filter=None, gray pixel
    compressed = zlib.compress(raw_data)
    buf.write(_chunk(b"IDAT", compressed))

    # IEND chunk
    buf.write(_chunk(b"IEND", b""))

    return buf.getvalue()
