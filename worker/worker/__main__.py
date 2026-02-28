"""Entry point for running the worker as a module: python3 -m worker."""

import os

from worker.server import serve

if __name__ == "__main__":
    port = int(os.environ.get("WORKER_PORT", "50051"))
    serve(port)
