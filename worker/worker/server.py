"""gRPC server for the HiveMind inference worker."""

import structlog

logger = structlog.get_logger("worker.server")


def serve(port: int = 50051) -> None:
    """Start the gRPC worker server."""
    logger.info("worker server starting", port=port)
    # TODO: Phase 4C — implement gRPC server
    logger.info("worker server placeholder — gRPC not yet implemented")
