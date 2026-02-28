package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/models"
)

// GRPCInferenceService implements InferenceService by delegating to the Python worker via gRPC.
type GRPCInferenceService struct {
	client  func() workerpb.WorkerServiceClient
	roomSvc RoomService
}

// NewGRPCInferenceService creates an inference service connected to the Python worker.
// clientFn is a function that returns the current gRPC client (allows reconnection).
func NewGRPCInferenceService(clientFn func() workerpb.WorkerServiceClient, roomSvc RoomService) *GRPCInferenceService {
	return &GRPCInferenceService{
		client:  clientFn,
		roomSvc: roomSvc,
	}
}

// ChatCompletion performs a non-streaming chat completion via the Python worker.
func (s *GRPCInferenceService) ChatCompletion(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	client := s.client()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	grpcReq := &workerpb.ChatRequest{
		RequestId:   fmt.Sprintf("hm-%d", time.Now().UnixNano()),
		Model:       req.Model,
		Temperature: float32(req.Temperature),
		MaxTokens:   int32(req.MaxTokens),
	}

	for _, msg := range req.Messages {
		grpcReq.Messages = append(grpcReq.Messages, &workerpb.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := client.ChatCompletion(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("worker chat completion failed: %w", err)
	}

	chatResp := models.NewChatResponse(grpcReq.RequestId, req.Model, resp.Content)
	if resp.Usage != nil {
		chatResp.Usage = models.UsageStats{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		}
	}

	return &chatResp, nil
}

// ChatCompletionStream performs a streaming chat completion, sending chunks to the channel.
func (s *GRPCInferenceService) ChatCompletionStream(ctx context.Context, req models.ChatRequest, ch chan<- models.ChatChunk) error {
	defer close(ch)

	client := s.client()
	if client == nil {
		return models.ErrWorkerUnavail
	}

	grpcReq := &workerpb.ChatRequest{
		RequestId:   fmt.Sprintf("hm-%d", time.Now().UnixNano()),
		Model:       req.Model,
		Temperature: float32(req.Temperature),
		MaxTokens:   int32(req.MaxTokens),
		Stream:      true,
	}

	for _, msg := range req.Messages {
		grpcReq.Messages = append(grpcReq.Messages, &workerpb.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	stream, err := client.ChatCompletionStream(ctx, grpcReq)
	if err != nil {
		return fmt.Errorf("worker stream failed: %w", err)
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("stream recv error: %w", err)
		}

		if chunk.Done {
			break
		}

		finishReason := (*string)(nil)
		modelChunk := models.ChatChunk{
			ID:      grpcReq.RequestId,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []models.ChatChunkChoice{
				{
					Index: 0,
					Delta: models.ChatChunkDelta{
						Content: chunk.Delta,
					},
					FinishReason: finishReason,
				},
			},
		}

		select {
		case ch <- modelChunk:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// ImageGeneration generates an image via the Python worker.
func (s *GRPCInferenceService) ImageGeneration(ctx context.Context, req models.ImageRequest) (*models.ImageResponse, error) {
	client := s.client()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	width, height := parseSize(req.Size)

	grpcReq := &workerpb.ImageRequest{
		RequestId:     fmt.Sprintf("hm-img-%d", time.Now().UnixNano()),
		Model:         req.Model,
		Prompt:        req.Prompt,
		Width:         int32(width),
		Height:        int32(height),
		Steps:         30,
		GuidanceScale: 7.5,
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	resp, err := client.ImageGeneration(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("worker image generation failed: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(resp.ImageData)

	return &models.ImageResponse{
		Created: time.Now().Unix(),
		Data: []models.ImageData{
			{B64JSON: b64},
		},
	}, nil
}

// ListModels returns the list of models available in the current room.
func (s *GRPCInferenceService) ListModels(ctx context.Context) (*models.ModelList, error) {
	room := s.roomSvc.CurrentRoom()
	if room == nil {
		return &models.ModelList{
			Object: "list",
			Data:   []models.ModelInfo{},
		}, nil
	}

	return &models.ModelList{
		Object: "list",
		Data: []models.ModelInfo{
			{
				ID:      room.ModelID,
				Object:  "model",
				OwnedBy: "hivemind-room",
			},
		},
	}, nil
}

// parseSize parses "1024x1024" into width and height.
func parseSize(size string) (int, int) {
	var w, h int
	if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil {
		return 1024, 1024
	}
	return w, h
}
