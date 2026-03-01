"""Tests for forward pass with mock model."""

import pytest
from worker.inference.llm import LLMEngine


def test_forward_pass_identity_mock():
    """Forward pass returns identity when model is None (mock mode)."""
    engine = LLMEngine()
    engine.is_ready = True
    # model is None — should pass through
    data = b"test_tensor_data"

    class MockMeta:
        from_layer = 0
        to_layer = 3

    result = engine.forward_pass(data, MockMeta(), compressed=False)
    assert result == data


def test_get_transformer_layers_no_model():
    """Returns empty list when no model loaded."""
    engine = LLMEngine()
    layers = engine._get_transformer_layers()
    assert layers == []
