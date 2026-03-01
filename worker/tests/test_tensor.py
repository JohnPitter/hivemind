"""Tests for tensor serialization round-trip."""

import numpy as np
import pytest
from worker.tensor.transfer import serialize_tensor, deserialize_tensor


def test_roundtrip_float32():
    arr = np.random.randn(2, 3, 4).astype(np.float32)
    data = serialize_tensor(arr)
    result = deserialize_tensor(data)
    # Convert back to numpy if torch tensor
    if hasattr(result, "numpy"):
        result = result.numpy()
    np.testing.assert_array_almost_equal(arr, result)


def test_roundtrip_float16():
    arr = np.random.randn(4, 8).astype(np.float16)
    data = serialize_tensor(arr)
    result = deserialize_tensor(data)
    if hasattr(result, "numpy"):
        result = result.numpy()
    np.testing.assert_array_almost_equal(arr, result)


def test_roundtrip_1d():
    arr = np.array([1.0, 2.0, 3.0], dtype=np.float32)
    data = serialize_tensor(arr)
    result = deserialize_tensor(data)
    if hasattr(result, "numpy"):
        result = result.numpy()
    np.testing.assert_array_equal(arr, result)


def test_roundtrip_large():
    arr = np.random.randn(64, 768).astype(np.float32)
    data = serialize_tensor(arr)
    result = deserialize_tensor(data)
    if hasattr(result, "numpy"):
        result = result.numpy()
    np.testing.assert_array_almost_equal(arr, result)


def test_preserves_dtype():
    for dtype in [np.float32, np.float64, np.float16]:
        arr = np.ones((2, 2), dtype=dtype)
        data = serialize_tensor(arr)
        result = deserialize_tensor(data)
        if hasattr(result, "numpy"):
            result = result.numpy()
        assert result.dtype == dtype
