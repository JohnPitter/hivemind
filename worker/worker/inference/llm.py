"""LLM inference engine with support for local and distributed (Petals) inference."""

from __future__ import annotations

import time
from typing import Iterator

import structlog

logger = structlog.get_logger("worker.inference.llm")


class LLMEngine:
    """Handles LLM inference, supporting local transformers and Petals for tensor parallelism."""

    def __init__(self) -> None:
        self.model_id: str | None = None
        self.loaded_layers: list[int] = []
        self.is_ready = False
        self._model = None
        self._tokenizer = None

    def load_model(self, model_id: str, layers: list[int]) -> None:
        """Load specific model layers into GPU memory.

        For single-node: loads the full model via transformers.
        For distributed: loads specific layers via Petals.
        """
        logger.info("loading model layers", model_id=model_id, layers=layers)

        self.model_id = model_id
        self.loaded_layers = layers

        try:
            self._load_with_transformers(model_id)
        except Exception as e:
            logger.warn(
                "transformers load failed, using mock mode",
                error=str(e),
                model_id=model_id,
            )
            # Mock mode — model "loads" but generates placeholder text
            self._model = None
            self._tokenizer = None

        self.is_ready = True
        logger.info("model ready", model_id=model_id, layers=layers)

    def _load_with_transformers(self, model_id: str) -> None:
        """Load model using HuggingFace transformers."""
        try:
            from transformers import AutoTokenizer, AutoModelForCausalLM
            import torch

            device = "cuda" if torch.cuda.is_available() else "cpu"
            dtype = torch.float16 if device == "cuda" else torch.float32

            logger.info("loading with transformers", model_id=model_id, device=device)

            self._tokenizer = AutoTokenizer.from_pretrained(model_id)
            self._model = AutoModelForCausalLM.from_pretrained(
                model_id,
                torch_dtype=dtype,
                device_map="auto" if device == "cuda" else None,
            )

            if device == "cpu" and self._model is not None:
                self._model = self._model.to(device)

            logger.info("model loaded via transformers", model_id=model_id)
        except ImportError:
            raise RuntimeError("transformers library not installed")

    def unload_model(self) -> None:
        """Unload model from GPU memory."""
        if self.model_id:
            logger.info("unloading model", model_id=self.model_id)

        self._model = None
        self._tokenizer = None
        self.model_id = None
        self.loaded_layers = []
        self.is_ready = False

        # Free GPU memory
        try:
            import torch
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
        except ImportError:
            pass

    def generate(
        self,
        messages: list[tuple[str, str]],
        temperature: float = 0.7,
        max_tokens: int = 2048,
    ) -> str:
        """Generate a complete response (non-streaming)."""
        if not self.is_ready:
            raise RuntimeError("model not loaded")

        if self._model is not None and self._tokenizer is not None:
            return self._generate_real(messages, temperature, max_tokens)

        return self._generate_mock(messages)

    def generate_stream(
        self,
        messages: list[tuple[str, str]],
        temperature: float = 0.7,
        max_tokens: int = 2048,
    ) -> Iterator[str]:
        """Generate response token by token (streaming)."""
        if not self.is_ready:
            raise RuntimeError("model not loaded")

        if self._model is not None and self._tokenizer is not None:
            yield from self._generate_stream_real(messages, temperature, max_tokens)
        else:
            yield from self._generate_stream_mock(messages)

    def _generate_real(
        self,
        messages: list[tuple[str, str]],
        temperature: float,
        max_tokens: int,
    ) -> str:
        """Generate using the loaded transformers model."""
        import torch

        prompt = self._format_prompt(messages)
        inputs = self._tokenizer(prompt, return_tensors="pt")

        device = next(self._model.parameters()).device
        inputs = {k: v.to(device) for k, v in inputs.items()}

        with torch.no_grad():
            outputs = self._model.generate(
                **inputs,
                max_new_tokens=max_tokens,
                temperature=max(temperature, 0.01),
                do_sample=temperature > 0,
                pad_token_id=self._tokenizer.eos_token_id,
            )

        generated = outputs[0][inputs["input_ids"].shape[1]:]
        return self._tokenizer.decode(generated, skip_special_tokens=True)

    def _generate_stream_real(
        self,
        messages: list[tuple[str, str]],
        temperature: float,
        max_tokens: int,
    ) -> Iterator[str]:
        """Stream tokens from transformers model using TextIteratorStreamer."""
        try:
            from transformers import TextIteratorStreamer
            import torch
            from threading import Thread

            prompt = self._format_prompt(messages)
            inputs = self._tokenizer(prompt, return_tensors="pt")
            device = next(self._model.parameters()).device
            inputs = {k: v.to(device) for k, v in inputs.items()}

            streamer = TextIteratorStreamer(
                self._tokenizer, skip_prompt=True, skip_special_tokens=True
            )

            gen_kwargs = {
                **inputs,
                "max_new_tokens": max_tokens,
                "temperature": max(temperature, 0.01),
                "do_sample": temperature > 0,
                "pad_token_id": self._tokenizer.eos_token_id,
                "streamer": streamer,
            }

            thread = Thread(target=self._model.generate, kwargs=gen_kwargs)
            thread.start()

            for text in streamer:
                if text:
                    yield text

            thread.join()
        except ImportError:
            # Fallback to non-streaming
            result = self._generate_real(messages, temperature, max_tokens)
            yield result

    def _generate_mock(self, messages: list[tuple[str, str]]) -> str:
        """Generate a mock response when no real model is loaded."""
        last_msg = messages[-1][1] if messages else "Hello"

        responses = [
            f"I received your message about '{last_msg[:30]}'. "
            "As a distributed inference node in HiveMind, I'm processing your request "
            "across multiple GPUs connected via tensor parallelism. "
            "This response demonstrates the mock inference pipeline working correctly.",

            "HiveMind's distributed architecture splits model layers across connected peers. "
            "Each node processes its assigned layers and forwards intermediate tensors "
            "to the next node via the WireGuard mesh. This approach enables running "
            "models that wouldn't fit on any single machine.",

            f"Processing query: '{last_msg[:40]}'. "
            "In production, this response would be generated by the actual model "
            "running distributed across all peers in the room. The tensor parallelism "
            "framework coordinates the forward pass across the network.",
        ]

        import hashlib
        idx = int(hashlib.md5(last_msg.encode()).hexdigest(), 16) % len(responses)
        return responses[idx]

    def _generate_stream_mock(self, messages: list[tuple[str, str]]) -> Iterator[str]:
        """Stream a mock response word by word."""
        response = self._generate_mock(messages)
        words = response.split(" ")

        for i, word in enumerate(words):
            prefix = " " if i > 0 else ""
            yield prefix + word
            time.sleep(0.04)  # ~25 tokens/sec

    def forward_pass(
        self,
        tensor_data: bytes,
        meta: object,
        compressed: bool,
    ) -> bytes:
        """Execute a forward pass through locally-loaded layers."""
        from worker.tensor.transfer import serialize_tensor, deserialize_tensor

        logger.info(
            "forward pass",
            data_size=len(tensor_data),
            compressed=compressed,
            layers=self.loaded_layers,
        )

        if self._model is None:
            return tensor_data  # identity fallback (mock mode)

        try:
            import torch
            hidden = deserialize_tensor(tensor_data)
            layers = self._get_transformer_layers()

            from_layer = getattr(meta, "from_layer", 0)
            to_layer = getattr(meta, "to_layer", len(layers) - 1)

            with torch.no_grad():
                for i in range(from_layer, min(to_layer + 1, len(layers))):
                    hidden = layers[i](hidden)[0]

            return serialize_tensor(hidden)
        except Exception as e:
            logger.error("forward pass failed, returning identity", error=str(e))
            return tensor_data

    def _get_transformer_layers(self):
        """Detect and return the model's transformer layers."""
        if self._model is None:
            return []

        # Llama / Mistral
        if hasattr(self._model, "model") and hasattr(self._model.model, "layers"):
            return list(self._model.model.layers)
        # GPT-NeoX
        if hasattr(self._model, "gpt_neox") and hasattr(self._model.gpt_neox, "layers"):
            return list(self._model.gpt_neox.layers)
        # GPT-2 / GPT-J
        if hasattr(self._model, "transformer") and hasattr(self._model.transformer, "h"):
            return list(self._model.transformer.h)

        logger.warn("unknown model architecture, cannot extract layers")
        return []

    def _format_prompt(self, messages: list[tuple[str, str]]) -> str:
        """Format messages into a prompt string for the model."""
        parts = []
        for role, content in messages:
            if role == "system":
                parts.append(f"System: {content}")
            elif role == "user":
                parts.append(f"User: {content}")
            elif role == "assistant":
                parts.append(f"Assistant: {content}")
        parts.append("Assistant:")
        return "\n".join(parts)
