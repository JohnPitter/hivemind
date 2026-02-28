package services

import (
	"context"

	"github.com/joaopedro/hivemind/internal/models"
)

// MockWorkerService simulates the Python inference worker.
type MockWorkerService struct {
	healthy bool
}

// NewMockWorkerService creates a mock worker service.
func NewMockWorkerService() *MockWorkerService {
	return &MockWorkerService{healthy: true}
}

func (s *MockWorkerService) Start(_ context.Context) error {
	s.healthy = true
	return nil
}

func (s *MockWorkerService) Stop(_ context.Context) error {
	s.healthy = false
	return nil
}

func (s *MockWorkerService) IsHealthy() bool {
	return s.healthy
}

func (s *MockWorkerService) GetResources() (*models.ResourceSpec, error) {
	return &models.ResourceSpec{
		GPUName:   "NVIDIA RTX 3060",
		VRAMTotal: 12288,
		VRAMFree:  10240,
		RAMTotal:  32768,
		RAMFree:   24576,
		CUDAAvail: true,
		Platform:  "Windows",
	}, nil
}
