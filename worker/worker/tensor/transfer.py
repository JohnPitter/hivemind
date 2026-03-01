"""Tensor serialization and transfer via gRPC."""

import struct
import structlog
import numpy as np

logger = structlog.get_logger("worker.tensor.transfer")

# Wire format:
# Header: dtype_len(1B) + dtype(NB) + ndim(4B) + shape(ndim*8B)
# Body: raw numpy bytes

def serialize_tensor(tensor) -> bytes:
    """Serialize a tensor (torch.Tensor or np.ndarray) to bytes."""
    # Convert torch.Tensor to numpy if needed
    arr = _to_numpy(tensor)

    dtype_str = str(arr.dtype).encode("utf-8")
    header = struct.pack("B", len(dtype_str))
    header += dtype_str
    header += struct.pack("<I", arr.ndim)
    for dim in arr.shape:
        header += struct.pack("<Q", dim)

    return header + arr.tobytes()


def deserialize_tensor(data: bytes):
    """Deserialize bytes back to a tensor.

    Returns torch.Tensor if torch is available, else np.ndarray.
    """
    offset = 0

    # Read dtype
    dtype_len = struct.unpack_from("B", data, offset)[0]
    offset += 1
    dtype_str = data[offset:offset + dtype_len].decode("utf-8")
    offset += dtype_len

    # Read ndim and shape
    ndim = struct.unpack_from("<I", data, offset)[0]
    offset += 4
    shape = []
    for _ in range(ndim):
        dim = struct.unpack_from("<Q", data, offset)[0]
        offset += 8
        shape.append(dim)

    # Read body
    arr = np.frombuffer(data[offset:], dtype=np.dtype(dtype_str)).reshape(shape)

    # Try to return as torch.Tensor
    try:
        import torch
        return torch.from_numpy(arr.copy())
    except ImportError:
        return arr.copy()


def _to_numpy(tensor) -> np.ndarray:
    """Convert tensor to numpy array."""
    if isinstance(tensor, np.ndarray):
        return tensor

    try:
        import torch
        if isinstance(tensor, torch.Tensor):
            return tensor.detach().cpu().numpy()
    except ImportError:
        pass

    return np.asarray(tensor)
