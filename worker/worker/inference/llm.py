"""LLM inference engine with support for local and distributed (Petals) inference."""

from __future__ import annotations

import time
from typing import Iterator

import structlog

logger = structlog.get_logger("worker.inference.llm")


class LLMEngine:
    """Handles LLM inference, supporting local transformers and Petals for tensor parallelism."""

    # TTL for KV cache entries (5 minutes of inactivity)
    _KV_CACHE_TTL_SECONDS = 300

    def __init__(self) -> None:
        self.model_id: str | None = None
        self.loaded_layers: list[int] = []
        self.is_ready = False
        self._model = None
        self._tokenizer = None
        self._kv_caches: dict[str, tuple[list, float]] = {}  # request_id -> (cache, last_access)

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
        use_cache: bool = False,
        request_id: str = "",
        cache_seq_len: int = 0,
    ) -> tuple[bytes, int]:
        """Execute a forward pass through locally-loaded layers.

        When use_cache=True, reuses KV cache from previous steps for this request_id,
        avoiding O(n²) recomputation of attention in autoregressive generation.

        Returns (output_tensor_data, new_cache_seq_len).
        """
        from worker.tensor.transfer import serialize_tensor, deserialize_tensor

        logger.info(
            "forward pass",
            data_size=len(tensor_data),
            compressed=compressed,
            layers=self.loaded_layers,
            use_cache=use_cache,
            request_id=request_id,
            cache_seq_len=cache_seq_len,
        )

        # Cleanup stale caches periodically
        self._cleanup_stale_caches()

        if self._model is None:
            # Identity fallback (mock mode) — simulate seq_len increment
            new_seq_len = cache_seq_len + 1
            return tensor_data, new_seq_len

        try:
            import torch
            hidden = deserialize_tensor(tensor_data)
            layers = self._get_transformer_layers()

            from_layer = getattr(meta, "from_layer", 0)
            to_layer = getattr(meta, "to_layer", len(layers) - 1)

            # Get or create per-request KV cache
            past_kv = None
            if use_cache and request_id and request_id in self._kv_caches:
                past_kv = self._kv_caches[request_id][0]

            new_kv = []
            with torch.no_grad():
                for idx, i in enumerate(range(from_layer, min(to_layer + 1, len(layers)))):
                    layer = layers[i]
                    layer_past = past_kv[idx] if past_kv and idx < len(past_kv) else None

                    # Most transformer layers accept past_key_value and use_cache kwargs
                    try:
                        out = layer(hidden, past_key_value=layer_past, use_cache=True)
                        hidden = out[0]
                        if len(out) > 1 and out[1] is not None:
                            new_kv.append(out[1])  # (key, value) tuple
                        else:
                            new_kv.append(None)
                    except TypeError:
                        # Layer doesn't support cache kwargs — run without cache
                        out = layer(hidden)
                        hidden = out[0] if isinstance(out, tuple) else out
                        new_kv.append(None)

            # Store cache for this request
            if request_id and any(kv is not None for kv in new_kv):
                self._kv_caches[request_id] = (new_kv, time.time())

            new_seq_len = cache_seq_len + hidden.shape[1]
            return serialize_tensor(hidden), new_seq_len
        except Exception as e:
            logger.error("forward pass failed, returning identity", error=str(e))
            return tensor_data, cache_seq_len

    def clear_cache(self, request_id: str) -> None:
        """Remove KV cache for a completed request."""
        self._kv_caches.pop(request_id, None)

    def _cleanup_stale_caches(self) -> None:
        """Remove KV caches that haven't been accessed in TTL seconds."""
        now = time.time()
        stale = [
            rid for rid, (_, last_access) in self._kv_caches.items()
            if now - last_access > self._KV_CACHE_TTL_SECONDS
        ]
        for rid in stale:
            del self._kv_caches[rid]
            logger.info("cleaned stale KV cache", request_id=rid)

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

    def embed_tokens(
        self,
        text: str | None = None,
        token_ids: list[int] | None = None,
    ) -> tuple[bytes, list[int], int]:
        """Embed text or token IDs into hidden states.

        Returns (serialized_hidden_states, token_ids, vocab_size).
        """
        if not self.is_ready:
            raise RuntimeError("model not loaded")

        if self._model is not None and self._tokenizer is not None:
            return self._embed_real(text, token_ids)

        return self._embed_mock(text, token_ids)

    def _embed_real(
        self,
        text: str | None,
        token_ids: list[int] | None,
    ) -> tuple[bytes, list[int], int]:
        """Embed using the loaded transformers model."""
        import torch
        from worker.tensor.transfer import serialize_tensor

        device = next(self._model.parameters()).device

        if text is not None:
            tokens = self._tokenizer(text, return_tensors="pt")
            input_ids = tokens["input_ids"].to(device)
            ids_list = input_ids[0].tolist()
        elif token_ids is not None:
            input_ids = torch.tensor([token_ids], dtype=torch.long, device=device)
            ids_list = token_ids
        else:
            raise ValueError("either text or token_ids must be provided")

        # Get embedding layer
        embed_layer = self._get_embed_layer()
        if embed_layer is None:
            raise RuntimeError("cannot find embedding layer for this model architecture")

        with torch.no_grad():
            hidden_states = embed_layer(input_ids)

        vocab_size = self._get_vocab_size()
        return serialize_tensor(hidden_states), ids_list, vocab_size

    def _embed_mock(
        self,
        text: str | None,
        token_ids: list[int] | None,
    ) -> tuple[bytes, list[int], int]:
        """Mock embedding for testing without a real model."""
        import numpy as np
        from worker.tensor.transfer import serialize_tensor

        if text is not None:
            # Simulate tokenization: ~4 chars per token
            num_tokens = max(1, len(text) // 4)
            ids_list = list(range(100, 100 + num_tokens))
        elif token_ids is not None:
            ids_list = token_ids
            num_tokens = len(ids_list)
        else:
            ids_list = [100]
            num_tokens = 1

        hidden_dim = 4096  # Common hidden dim
        hidden_states = np.random.randn(1, num_tokens, hidden_dim).astype(np.float32)
        return serialize_tensor(hidden_states), ids_list, 32000

    def sample_tokens(
        self,
        logits_data: bytes,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 50,
        eos_token_ids: list[int] | None = None,
    ) -> tuple[int, str, bool, float]:
        """Sample the next token from logits.

        Returns (token_id, token_text, is_eos, token_probability).
        """
        if not self.is_ready:
            raise RuntimeError("model not loaded")

        if self._model is not None and self._tokenizer is not None:
            return self._sample_real(logits_data, temperature, top_p, top_k, eos_token_ids)

        return self._sample_mock(logits_data, temperature, eos_token_ids)

    def _sample_real(
        self,
        logits_data: bytes,
        temperature: float,
        top_p: float,
        top_k: int,
        eos_token_ids: list[int] | None,
    ) -> tuple[int, str, bool, float]:
        """Sample using the loaded model's lm_head and sampling logic."""
        import torch
        import torch.nn.functional as F
        from worker.tensor.transfer import deserialize_tensor

        device = next(self._model.parameters()).device
        hidden_states = deserialize_tensor(logits_data).to(device)

        # Apply lm_head to get logits
        lm_head = self._get_lm_head()
        if lm_head is None:
            raise RuntimeError("cannot find lm_head for this model architecture")

        with torch.no_grad():
            logits = lm_head(hidden_states)

        # Take logits for the last token: [1, seq_len, vocab] → [vocab]
        logits = logits[0, -1, :]

        # Apply temperature
        if temperature > 0:
            logits = logits / max(temperature, 1e-5)

        # Apply top-k filtering
        if top_k > 0:
            top_k = min(top_k, logits.size(-1))
            top_k_values, _ = torch.topk(logits, top_k)
            threshold = top_k_values[-1]
            logits[logits < threshold] = float("-inf")

        # Apply top-p (nucleus) filtering
        if 0 < top_p < 1.0:
            sorted_logits, sorted_indices = torch.sort(logits, descending=True)
            probs = F.softmax(sorted_logits, dim=-1)
            cumulative_probs = torch.cumsum(probs, dim=-1)
            mask = cumulative_probs - probs > top_p
            sorted_logits[mask] = float("-inf")
            logits = sorted_logits.scatter(0, sorted_indices, sorted_logits)

        # Sample
        probs = F.softmax(logits, dim=-1)
        if temperature > 0:
            token_id = torch.multinomial(probs, num_samples=1).item()
        else:
            token_id = torch.argmax(probs).item()

        token_prob = probs[token_id].item()
        token_text = self._tokenizer.decode([token_id])

        # Check EOS
        is_eos = False
        if eos_token_ids:
            is_eos = token_id in eos_token_ids
        elif self._tokenizer.eos_token_id is not None:
            is_eos = token_id == self._tokenizer.eos_token_id

        return token_id, token_text, is_eos, token_prob

    def _sample_mock(
        self,
        logits_data: bytes,
        temperature: float,
        eos_token_ids: list[int] | None,
    ) -> tuple[int, str, bool, float]:
        """Mock sampling for testing without a real model."""
        # Generate deterministic mock tokens based on a counter
        if not hasattr(self, "_mock_token_idx"):
            self._mock_token_idx = 0
            self._mock_words = [
                "The", " distributed", " inference", " pipeline", " is",
                " processing", " your", " request", " across", " multiple",
                " peers", " using", " tensor", " parallelism", ".",
            ]

        idx = self._mock_token_idx % len(self._mock_words)
        self._mock_token_idx += 1

        word = self._mock_words[idx]
        is_eos = idx == len(self._mock_words) - 1
        token_id = 1000 + idx

        return token_id, word, is_eos, 0.95

    def _get_embed_layer(self):
        """Get the model's token embedding layer."""
        if self._model is None:
            return None

        # Llama / Mistral
        if hasattr(self._model, "model") and hasattr(self._model.model, "embed_tokens"):
            return self._model.model.embed_tokens
        # GPT-NeoX
        if hasattr(self._model, "gpt_neox") and hasattr(self._model.gpt_neox, "embed_in"):
            return self._model.gpt_neox.embed_in
        # GPT-2 / GPT-J
        if hasattr(self._model, "transformer") and hasattr(self._model.transformer, "wte"):
            return self._model.transformer.wte

        logger.warn("unknown model architecture, cannot extract embedding layer")
        return None

    def _get_lm_head(self):
        """Get the model's language model head (output projection)."""
        if self._model is None:
            return None

        # Most models (Llama, Mistral, GPT-NeoX, GPT-2)
        if hasattr(self._model, "lm_head"):
            return self._model.lm_head

        logger.warn("unknown model architecture, cannot extract lm_head")
        return None

    def _get_vocab_size(self) -> int:
        """Get the model's vocabulary size."""
        if self._model is not None and hasattr(self._model, "config"):
            return getattr(self._model.config, "vocab_size", 32000)
        if self._tokenizer is not None:
            return len(self._tokenizer)
        return 32000

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
