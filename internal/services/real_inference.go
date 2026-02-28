package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// RealInferenceService implements InferenceService by delegating to the
// Python worker process via gRPC. The worker loads and runs the actual
// model on GPU or CPU.
type RealInferenceService struct {
	roomSvc RoomService
	wm      *infra.WorkerManager
	mu      sync.Mutex
	started bool
	loaded  bool
}

// NewRealInferenceService creates an inference service backed by the Python worker.
func NewRealInferenceService(roomSvc RoomService, wm *infra.WorkerManager) *RealInferenceService {
	return &RealInferenceService{
		roomSvc: roomSvc,
		wm:      wm,
	}
}

// ensureWorkerRunning starts the Python worker if not already running.
func (s *RealInferenceService) ensureWorkerRunning(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	logger.Info("starting Python inference worker", "service", "real_inference")

	// Use background context — worker must outlive individual HTTP requests
	if err := s.wm.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}

	s.started = true
	logger.Info("Python inference worker started", "service", "real_inference")
	return nil
}

// ensureModelLoaded loads the model into the worker when a room is active.
func (s *RealInferenceService) ensureModelLoaded(ctx context.Context) error {
	if err := s.ensureWorkerRunning(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded {
		return nil
	}

	room := s.roomSvc.CurrentRoom()
	if room == nil {
		return models.ErrNotInRoom
	}

	client := s.wm.Client()
	if client == nil {
		return models.ErrWorkerUnavail
	}

	// Build layer list from the local peer's assignment
	var layers []int32
	for _, peer := range room.Peers {
		for _, l := range peer.Layers {
			layers = append(layers, int32(l))
		}
	}

	// Determine model type
	modelType := workerpb.LoadModelRequest_LLM
	if room.ModelType == models.ModelTypeDiffusion {
		modelType = workerpb.LoadModelRequest_DIFFUSION
	}

	logger.Info("loading model into worker",
		"model_id", room.ModelID,
		"layers", len(layers),
		"model_type", modelType,
	)

	loadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := client.LoadModel(loadCtx, &workerpb.LoadModelRequest{
		ModelId:   room.ModelID,
		Layers:    layers,
		ModelType: modelType,
	})
	if err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("worker failed to load model: %s", resp.Error)
	}

	s.loaded = true
	logger.Info("model loaded successfully", "model_id", room.ModelID)
	return nil
}

// ChatCompletion runs a non-streaming chat completion via the Python worker.
func (s *RealInferenceService) ChatCompletion(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	room := s.roomSvc.CurrentRoom()
	if room == nil {
		return nil, models.ErrNotInRoom
	}

	if err := s.ensureModelLoaded(ctx); err != nil {
		return nil, err
	}

	client := s.wm.Client()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	requestID := fmt.Sprintf("hm-%s", generateID(6))

	// Convert models.ChatMessage → workerpb.ChatMessage
	grpcMessages := make([]*workerpb.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		grpcMessages[i] = &workerpb.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	temp := float32(req.Temperature)
	if temp == 0 {
		temp = 0.7
	}
	maxTokens := int32(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 256
	}

	grpcReq := &workerpb.ChatRequest{
		RequestId:   requestID,
		Model:       req.Model,
		Messages:    grpcMessages,
		Temperature: temp,
		MaxTokens:   maxTokens,
	}

	inferCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	grpcResp, err := client.ChatCompletion(inferCtx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("worker chat completion failed: %w", err)
	}

	// Convert workerpb.ChatResponse → models.ChatResponse
	resp := models.NewChatResponse(requestID, req.Model, grpcResp.Content)

	if grpcResp.Usage != nil {
		resp.Usage = models.UsageStats{
			PromptTokens:     int(grpcResp.Usage.PromptTokens),
			CompletionTokens: int(grpcResp.Usage.CompletionTokens),
			TotalTokens:      int(grpcResp.Usage.TotalTokens),
		}
	}

	return &resp, nil
}

// ChatCompletionStream runs a streaming chat completion via the Python worker.
func (s *RealInferenceService) ChatCompletionStream(ctx context.Context, req models.ChatRequest, ch chan<- models.ChatChunk) error {
	defer close(ch)

	room := s.roomSvc.CurrentRoom()
	if room == nil {
		return models.ErrNotInRoom
	}

	if err := s.ensureModelLoaded(ctx); err != nil {
		return err
	}

	client := s.wm.Client()
	if client == nil {
		return models.ErrWorkerUnavail
	}

	requestID := fmt.Sprintf("hm-%s", generateID(6))

	grpcMessages := make([]*workerpb.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		grpcMessages[i] = &workerpb.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	temp := float32(req.Temperature)
	if temp == 0 {
		temp = 0.7
	}
	maxTokens := int32(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 256
	}

	grpcReq := &workerpb.ChatRequest{
		RequestId:   requestID,
		Model:       req.Model,
		Messages:    grpcMessages,
		Temperature: temp,
		MaxTokens:   maxTokens,
	}

	stream, err := client.ChatCompletionStream(ctx, grpcReq)
	if err != nil {
		return fmt.Errorf("worker stream failed: %w", err)
	}

	// Send initial role chunk
	ch <- models.ChatChunk{
		ID:      requestID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []models.ChatChunkChoice{
			{
				Index: 0,
				Delta: models.ChatChunkDelta{Role: "assistant"},
			},
		},
	}

	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}

		if chunk.Done {
			finishReason := "stop"
			ch <- models.ChatChunk{
				ID:      requestID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []models.ChatChunkChoice{
					{
						Index:        0,
						Delta:        models.ChatChunkDelta{},
						FinishReason: &finishReason,
					},
				},
			}
			break
		}

		if chunk.Delta != "" {
			ch <- models.ChatChunk{
				ID:      requestID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []models.ChatChunkChoice{
					{
						Index: 0,
						Delta: models.ChatChunkDelta{Content: chunk.Delta},
					},
				},
			}
		}
	}

	return nil
}

// ImageGeneration runs image generation via the Python worker.
func (s *RealInferenceService) ImageGeneration(ctx context.Context, req models.ImageRequest) (*models.ImageResponse, error) {
	room := s.roomSvc.CurrentRoom()
	if room == nil {
		return nil, models.ErrNotInRoom
	}

	if err := s.ensureModelLoaded(ctx); err != nil {
		return nil, err
	}

	client := s.wm.Client()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	requestID := fmt.Sprintf("hm-%s", generateID(6))

	n := int32(1)
	if req.N > 0 {
		n = int32(req.N)
	}

	grpcReq := &workerpb.ImageRequest{
		RequestId: requestID,
		Model:     req.Model,
		Prompt:    req.Prompt,
		Width:     1024,
		Height:    1024,
	}

	inferCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	grpcResp, err := client.ImageGeneration(inferCtx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("worker image generation failed: %w", err)
	}

	imageData := make([]models.ImageData, n)
	b64 := base64.StdEncoding.EncodeToString(grpcResp.ImageData)
	for i := range imageData {
		imageData[i] = models.ImageData{B64JSON: b64}
	}

	return &models.ImageResponse{
		Created: time.Now().Unix(),
		Data:    imageData,
	}, nil
}

// ListModels returns the models available in the current room.
func (s *RealInferenceService) ListModels(_ context.Context) (*models.ModelList, error) {
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

// ResetModelState clears the loaded flag so model is reloaded on next call.
// Used when room changes (leave/join).
func (s *RealInferenceService) ResetModelState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = false
}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}
