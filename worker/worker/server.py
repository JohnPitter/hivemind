"""gRPC server for the HiveMind inference worker."""

from __future__ import annotations

import time
import uuid
from concurrent import futures

import grpc
import structlog

from worker.gen import worker_pb2, worker_pb2_grpc
from worker.inference.llm import LLMEngine
from worker.inference.diffusion import DiffusionEngine
from worker.resources.detector import detect, ResourceInfo

logger = structlog.get_logger("worker.server")


class WorkerServicer(worker_pb2_grpc.WorkerServiceServicer):
    """Implements the WorkerService gRPC interface."""

    def __init__(self) -> None:
        self.llm_engine = LLMEngine()
        self.diffusion_engine = DiffusionEngine()
        self.resources: ResourceInfo | None = None
        self.state = worker_pb2.StatusResponse.IDLE
        self.model_id: str | None = None
        self.loaded_layers: list[int] = []
        self._detect_resources()

    def _detect_resources(self) -> None:
        """Detect and cache system resources."""
        try:
            self.resources = detect()
        except Exception as e:
            logger.error("failed to detect resources", error=str(e))
            self.resources = ResourceInfo(
                gpu_name="Unknown",
                vram_total_mb=0,
                vram_free_mb=0,
                ram_total_mb=0,
                ram_free_mb=0,
                cuda_available=False,
                platform="unknown",
            )

    def _resource_usage(self) -> worker_pb2.ResourceUsage:
        """Build a ResourceUsage protobuf from detected resources."""
        if self.resources is None:
            return worker_pb2.ResourceUsage()
        return worker_pb2.ResourceUsage(
            vram_total_mb=self.resources.vram_total_mb,
            vram_used_mb=self.resources.vram_total_mb - self.resources.vram_free_mb,
            ram_total_mb=self.resources.ram_total_mb,
            ram_used_mb=self.resources.ram_total_mb - self.resources.ram_free_mb,
            gpu_name=self.resources.gpu_name,
        )

    def LoadModel(self, request, context):
        """Load a model (or specific layers) into GPU/CPU memory."""
        logger.info(
            "LoadModel request",
            model_id=request.model_id,
            layers=list(request.layers),
            model_type=request.model_type,
        )

        self.state = worker_pb2.StatusResponse.LOADING
        self.model_id = request.model_id
        self.loaded_layers = list(request.layers)

        try:
            if request.model_type == worker_pb2.LoadModelRequest.LLM:
                self.llm_engine.load_model(request.model_id, list(request.layers))
            else:
                self.diffusion_engine.load_stage(request.model_id, "full")

            self.state = worker_pb2.StatusResponse.READY
            logger.info("model loaded successfully", model_id=request.model_id)

            return worker_pb2.LoadModelResponse(
                success=True,
                resources_used=self._resource_usage(),
            )
        except Exception as e:
            self.state = worker_pb2.StatusResponse.ERROR
            logger.error("failed to load model", error=str(e))
            return worker_pb2.LoadModelResponse(
                success=False,
                error=str(e),
            )

    def UnloadModel(self, request, context):
        """Unload the current model from memory."""
        logger.info("UnloadModel request")

        self.llm_engine.unload_model()
        self.diffusion_engine.unload_stage()
        self.state = worker_pb2.StatusResponse.IDLE
        self.model_id = None
        self.loaded_layers = []

        return worker_pb2.UnloadModelResponse(success=True)

    def GetStatus(self, request, context):
        """Return current worker status and resource usage."""
        return worker_pb2.StatusResponse(
            state=self.state,
            model_id=self.model_id or "",
            loaded_layers=self.loaded_layers,
            resources=self._resource_usage(),
        )

    def ChatCompletion(self, request, context):
        """Run a non-streaming chat completion."""
        logger.info(
            "ChatCompletion request",
            request_id=request.request_id,
            model=request.model,
            num_messages=len(request.messages),
        )

        if self.state != worker_pb2.StatusResponse.READY:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("model not loaded")
            return worker_pb2.ChatResponse()

        self.state = worker_pb2.StatusResponse.PROCESSING

        try:
            # For now, use mock response (real inference in Phase 6)
            content = self.llm_engine.generate(
                messages=[(m.role, m.content) for m in request.messages],
                temperature=request.temperature,
                max_tokens=request.max_tokens,
            )

            self.state = worker_pb2.StatusResponse.READY

            return worker_pb2.ChatResponse(
                request_id=request.request_id,
                content=content,
                usage=worker_pb2.UsageStats(
                    prompt_tokens=len(str(request.messages)) // 4,
                    completion_tokens=len(content) // 4,
                    total_tokens=(len(str(request.messages)) + len(content)) // 4,
                ),
            )
        except Exception as e:
            self.state = worker_pb2.StatusResponse.READY
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.ChatResponse()

    def ChatCompletionStream(self, request, context):
        """Run a streaming chat completion, yielding token-by-token."""
        logger.info(
            "ChatCompletionStream request",
            request_id=request.request_id,
            model=request.model,
        )

        if self.state != worker_pb2.StatusResponse.READY:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("model not loaded")
            return

        self.state = worker_pb2.StatusResponse.PROCESSING

        try:
            for token in self.llm_engine.generate_stream(
                messages=[(m.role, m.content) for m in request.messages],
                temperature=request.temperature,
                max_tokens=request.max_tokens,
            ):
                yield worker_pb2.ChatChunk(
                    request_id=request.request_id,
                    delta=token,
                    done=False,
                )

            # Final chunk with done=True
            yield worker_pb2.ChatChunk(
                request_id=request.request_id,
                delta="",
                done=True,
            )
        except Exception as e:
            logger.error("streaming error", error=str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
        finally:
            self.state = worker_pb2.StatusResponse.READY

    def ImageGeneration(self, request, context):
        """Generate an image from a text prompt."""
        logger.info(
            "ImageGeneration request",
            request_id=request.request_id,
            model=request.model,
            prompt=request.prompt[:50],
        )

        if self.state != worker_pb2.StatusResponse.READY:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("model not loaded")
            return worker_pb2.ImageResponse()

        self.state = worker_pb2.StatusResponse.PROCESSING

        try:
            image_data = self.diffusion_engine.generate(
                prompt=request.prompt,
                width=request.width or 1024,
                height=request.height or 1024,
                steps=request.steps or 30,
                guidance_scale=request.guidance_scale or 7.5,
            )

            self.state = worker_pb2.StatusResponse.READY

            return worker_pb2.ImageResponse(
                request_id=request.request_id,
                image_data=image_data,
                format="png",
            )
        except Exception as e:
            self.state = worker_pb2.StatusResponse.READY
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.ImageResponse()

    def ForwardPass(self, request, context):
        """Execute a forward pass through local model layers (tensor parallelism)."""
        logger.info(
            "ForwardPass request",
            request_id=request.request_id,
            compressed=request.compressed,
        )

        start = time.monotonic()

        try:
            # Decompress if needed, run through local layers, compress result
            output_data = self.llm_engine.forward_pass(
                tensor_data=request.tensor_data,
                meta=request.meta,
                compressed=request.compressed,
            )

            duration = (time.monotonic() - start) * 1000

            return worker_pb2.ForwardResponse(
                request_id=request.request_id,
                tensor_data=output_data,
                meta=request.meta,
                compressed=request.compressed,
                duration_ms=duration,
            )
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.ForwardResponse()


def serve(port: int = 50051) -> None:
    """Start the gRPC worker server."""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    servicer = WorkerServicer()
    worker_pb2_grpc.add_WorkerServiceServicer_to_server(servicer, server)

    addr = f"[::]:{port}"
    server.add_insecure_port(addr)

    logger.info("worker gRPC server starting", address=addr)
    server.start()
    logger.info("worker gRPC server started", address=addr)

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("shutting down worker server")
        server.stop(grace=5)
