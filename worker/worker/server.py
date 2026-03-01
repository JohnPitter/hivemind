"""gRPC server for the HiveMind inference worker."""

from __future__ import annotations

print("[worker] Python process started", flush=True)

import time
from concurrent import futures

print("[worker] importing grpc...", flush=True)
import grpc
print(f"[worker] grpc version: {grpc.__version__}", flush=True)

import structlog

print("[worker] importing generated protobuf...", flush=True)
from worker.gen import worker_pb2, worker_pb2_grpc
print("[worker] protobuf imported OK", flush=True)

print("[worker] importing engines...", flush=True)
from worker.inference.llm import LLMEngine
from worker.inference.diffusion import DiffusionEngine
from worker.resources.detector import detect, ResourceInfo
print("[worker] all imports done", flush=True)

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
            use_cache=request.use_cache,
            cache_seq_len=request.cache_seq_len,
        )

        start = time.monotonic()

        try:
            output_data, new_seq_len = self.llm_engine.forward_pass(
                tensor_data=request.tensor_data,
                meta=request.meta,
                compressed=request.compressed,
                use_cache=request.use_cache,
                request_id=request.request_id,
                cache_seq_len=request.cache_seq_len,
            )

            duration = (time.monotonic() - start) * 1000

            return worker_pb2.ForwardResponse(
                request_id=request.request_id,
                tensor_data=output_data,
                meta=request.meta,
                compressed=request.compressed,
                duration_ms=duration,
                cache_seq_len=new_seq_len,
            )
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.ForwardResponse()

    def EmbedTokens(self, request, context):
        """Embed text or token IDs into hidden states for distributed generation."""
        logger.info(
            "EmbedTokens request",
            request_id=request.request_id,
            has_text=bool(request.text),
            num_token_ids=len(request.token_ids),
        )

        if self.state != worker_pb2.StatusResponse.READY:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("model not loaded")
            return worker_pb2.EmbedResponse()

        try:
            text = request.text if request.text else None
            token_ids = list(request.token_ids) if request.token_ids else None

            hidden_states, ids_list, vocab_size = self.llm_engine.embed_tokens(
                text=text,
                token_ids=token_ids,
            )

            return worker_pb2.EmbedResponse(
                request_id=request.request_id,
                hidden_states=hidden_states,
                token_ids=ids_list,
                vocab_size=vocab_size,
            )
        except Exception as e:
            logger.error("embed_tokens failed", error=str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.EmbedResponse()

    def SampleTokens(self, request, context):
        """Sample the next token from logits/hidden states."""
        logger.info(
            "SampleTokens request",
            request_id=request.request_id,
            temperature=request.temperature,
        )

        if self.state != worker_pb2.StatusResponse.READY:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("model not loaded")
            return worker_pb2.SampleResponse()

        try:
            eos_ids = list(request.eos_token_ids) if request.eos_token_ids else None

            token_id, token_text, is_eos, token_prob = self.llm_engine.sample_tokens(
                logits_data=request.logits,
                temperature=request.temperature if request.temperature > 0 else 0.7,
                top_p=request.top_p if request.top_p > 0 else 0.9,
                top_k=request.top_k if request.top_k > 0 else 50,
                eos_token_ids=eos_ids,
            )

            return worker_pb2.SampleResponse(
                request_id=request.request_id,
                token_id=token_id,
                token_text=token_text,
                is_eos=is_eos,
                token_prob=token_prob,
            )
        except Exception as e:
            logger.error("sample_tokens failed", error=str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return worker_pb2.SampleResponse()


def serve(port: int = 50051) -> None:
    """Start the gRPC worker server."""
    print(f"[worker] serve() called, port={port}", flush=True)
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    print("[worker] creating WorkerServicer...", flush=True)
    servicer = WorkerServicer()
    print("[worker] WorkerServicer created", flush=True)
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
