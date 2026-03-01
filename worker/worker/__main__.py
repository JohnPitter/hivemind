"""Entry point for running the worker as a module: python3 -m worker."""

import os
import sys

# Add the gen/ directory to sys.path so that protobuf-generated files
# (which use absolute imports like `import worker_pb2`) can be resolved.
_gen_dir = os.path.join(os.path.dirname(__file__), "gen")
if _gen_dir not in sys.path:
    sys.path.insert(0, _gen_dir)

from worker.server import serve

if __name__ == "__main__":
    port = int(os.environ.get("WORKER_PORT", "50051"))
    serve(port)
