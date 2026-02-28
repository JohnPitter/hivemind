package services

import (
	"context"

	"github.com/joaopedro/hivemind/internal/models"
)

// RoomService manages room lifecycle.
type RoomService interface {
	Create(ctx context.Context, cfg models.RoomConfig) (*models.Room, error)
	Join(ctx context.Context, inviteCode string, resources models.ResourceSpec) (*models.Room, error)
	Leave(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (*models.RoomStatus, error)
	CurrentRoom() *models.Room
}

// InferenceService handles AI inference routing.
type InferenceService interface {
	ChatCompletion(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error)
	ChatCompletionStream(ctx context.Context, req models.ChatRequest, ch chan<- models.ChatChunk) error
	ImageGeneration(ctx context.Context, req models.ImageRequest) (*models.ImageResponse, error)
	ListModels(ctx context.Context) (*models.ModelList, error)
}

// WorkerService manages the local Python inference worker.
type WorkerService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsHealthy() bool
	GetResources() (*models.ResourceSpec, error)
}
