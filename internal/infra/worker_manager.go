package infra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/logger"
)

// WorkerManager spawns and manages the Python inference worker process.
// It handles health checks, auto-restart with exponential backoff, and
// provides a gRPC client to communicate with the worker.
type WorkerManager struct {
	mu           sync.RWMutex
	cmd          *exec.Cmd
	grpcConn     *grpc.ClientConn
	grpcClient   workerpb.WorkerServiceClient
	port         int
	pythonCmd    string
	workerDir    string
	isRunning    bool
	restartCount int
	maxRestarts  int
	cancel       context.CancelFunc
}

// WorkerConfig configures the worker manager.
type WorkerConfig struct {
	Port      int    // gRPC port (default: 50051)
	PythonCmd string // Python executable (default: "python")
	WorkerDir string // Worker directory (default: "worker")
	MaxRestarts int  // Max restart attempts (default: 5)
}

// DefaultWorkerConfig returns sane defaults.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		Port:        50051,
		PythonCmd:   "python",
		WorkerDir:   "worker",
		MaxRestarts: 5,
	}
}

// NewWorkerManager creates a worker manager with the given config.
func NewWorkerManager(cfg WorkerConfig) *WorkerManager {
	return &WorkerManager{
		port:        cfg.Port,
		pythonCmd:   cfg.PythonCmd,
		workerDir:   cfg.WorkerDir,
		maxRestarts: cfg.MaxRestarts,
	}
}

// Start spawns the Python worker process and connects the gRPC client.
func (wm *WorkerManager) Start(ctx context.Context) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.isRunning {
		return fmt.Errorf("worker already running")
	}

	ctx, wm.cancel = context.WithCancel(ctx)

	if err := wm.spawnProcess(ctx); err != nil {
		return fmt.Errorf("failed to spawn worker: %w", err)
	}

	// Wait for worker to be ready
	if err := wm.waitForReady(ctx); err != nil {
		wm.killProcess()
		return fmt.Errorf("worker failed to start: %w", err)
	}

	wm.isRunning = true
	wm.restartCount = 0

	// Start health monitoring in background
	go wm.monitorHealth(ctx)

	logger.Info("worker started", "port", wm.port)
	return nil
}

// Stop gracefully shuts down the worker process.
func (wm *WorkerManager) Stop(_ context.Context) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.cancel != nil {
		wm.cancel()
	}

	if wm.grpcConn != nil {
		wm.grpcConn.Close()
		wm.grpcConn = nil
		wm.grpcClient = nil
	}

	wm.killProcess()
	wm.isRunning = false

	logger.Info("worker stopped")
	return nil
}

// IsHealthy returns whether the worker is running and responding.
func (wm *WorkerManager) IsHealthy() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if !wm.isRunning || wm.grpcClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := wm.grpcClient.GetStatus(ctx, &workerpb.StatusRequest{})
	return err == nil
}

// Client returns the gRPC client for communicating with the worker.
func (wm *WorkerManager) Client() workerpb.WorkerServiceClient {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.grpcClient
}

func (wm *WorkerManager) spawnProcess(ctx context.Context) error {
	wm.cmd = exec.CommandContext(ctx, wm.pythonCmd, "-m", "worker")
	wm.cmd.Dir = wm.workerDir
	wm.cmd.Env = append(os.Environ(),
		fmt.Sprintf("WORKER_PORT=%d", wm.port),
		"PYTHONUNBUFFERED=1",
	)
	wm.cmd.Stdout = os.Stdout
	wm.cmd.Stderr = os.Stderr

	if err := wm.cmd.Start(); err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	logger.Info("worker process spawned", "pid", wm.cmd.Process.Pid, "port", wm.port)
	return nil
}

func (wm *WorkerManager) waitForReady(ctx context.Context) error {
	addr := fmt.Sprintf("localhost:%d", wm.port)

	// Retry connection for up to 120 seconds (torch/transformers import can be slow)
	deadline := time.Now().Add(120 * time.Second)
	backoff := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			time.Sleep(backoff)
			backoff = min(backoff*2, 5*time.Second)
			continue
		}

		client := workerpb.NewWorkerServiceClient(conn)
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err = client.GetStatus(checkCtx, &workerpb.StatusRequest{})
		cancel()

		if err == nil {
			wm.grpcConn = conn
			wm.grpcClient = client
			return nil
		}

		conn.Close()
		time.Sleep(backoff)
		backoff = min(backoff*2, 5*time.Second)
	}

	return fmt.Errorf("worker did not become ready within timeout")
}

func (wm *WorkerManager) killProcess() {
	if wm.cmd != nil && wm.cmd.Process != nil {
		if err := wm.cmd.Process.Kill(); err != nil {
			logger.Warn("failed to kill worker process", "error", err)
		}
		wm.cmd.Wait()
		wm.cmd = nil
	}
}

func (wm *WorkerManager) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !wm.IsHealthy() {
				wm.handleUnhealthy(ctx)
			}
		}
	}
}

func (wm *WorkerManager) handleUnhealthy(ctx context.Context) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if !wm.isRunning {
		return
	}

	wm.restartCount++
	if wm.restartCount > wm.maxRestarts {
		logger.Error("worker exceeded max restarts, giving up", "count", wm.restartCount)
		wm.isRunning = false
		return
	}

	backoff := time.Duration(wm.restartCount) * 2 * time.Second
	logger.Warn("worker unhealthy, restarting",
		"attempt", wm.restartCount,
		"backoff", backoff.String(),
	)

	wm.killProcess()

	if wm.grpcConn != nil {
		wm.grpcConn.Close()
		wm.grpcConn = nil
		wm.grpcClient = nil
	}

	time.Sleep(backoff)

	if err := wm.spawnProcess(ctx); err != nil {
		logger.Error("failed to restart worker", "error", err)
		return
	}

	if err := wm.waitForReady(ctx); err != nil {
		logger.Error("restarted worker failed to become ready", "error", err)
		wm.killProcess()
		return
	}

	logger.Info("worker restarted successfully", "attempt", wm.restartCount)
}
