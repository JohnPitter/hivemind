package services

import (
	"context"

	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/models"
)

// RealWorkerService implements WorkerService using the WorkerManager.
type RealWorkerService struct {
	manager *infra.WorkerManager
}

// NewRealWorkerService creates a worker service backed by the process manager.
func NewRealWorkerService(manager *infra.WorkerManager) *RealWorkerService {
	return &RealWorkerService{manager: manager}
}

// Start spawns the Python worker process.
func (s *RealWorkerService) Start(ctx context.Context) error {
	return s.manager.Start(ctx)
}

// Stop kills the Python worker process.
func (s *RealWorkerService) Stop(ctx context.Context) error {
	return s.manager.Stop(ctx)
}

// IsHealthy checks if the worker is responding to health checks.
func (s *RealWorkerService) IsHealthy() bool {
	return s.manager.IsHealthy()
}

// GetResources returns the worker's detected GPU/RAM resources.
func (s *RealWorkerService) GetResources() (*models.ResourceSpec, error) {
	client := s.manager.Client()
	if client == nil {
		return nil, models.ErrWorkerUnavail
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5000000000) // 5s
	defer cancel()

	status, err := client.GetStatus(ctx, nil)
	if err != nil {
		return nil, err
	}

	if status.Resources == nil {
		return &models.ResourceSpec{}, nil
	}

	return &models.ResourceSpec{
		GPUName:   status.Resources.GpuName,
		VRAMTotal: status.Resources.VramTotalMb,
		VRAMFree:  status.Resources.VramUsedMb,
		RAMTotal:  status.Resources.RamTotalMb,
	}, nil
}
