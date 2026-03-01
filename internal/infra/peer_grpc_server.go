package infra

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/joaopedro/hivemind/gen/peerpb"
	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/logger"
)

// PeerGRPCServer implements the PeerService gRPC server for receiving
// requests from other peers in the mesh.
type PeerGRPCServer struct {
	peerpb.UnimplementedPeerServiceServer

	mu            sync.RWMutex
	localPeerID   string
	localWorker   func() workerpb.WorkerServiceClient
	roomToken     string
	stateVersion  uint64
	compressor    *TensorCompressor
	server        *grpc.Server
	onHandshake   func(peerID string, resources *workerpb.ResourceUsage)
	getRoomState  func() *peerpb.RoomState
}

// PeerGRPCServerConfig holds configuration for creating a peer gRPC server.
type PeerGRPCServerConfig struct {
	LocalPeerID  string
	LocalWorker  func() workerpb.WorkerServiceClient
	RoomToken    string
	OnHandshake  func(peerID string, resources *workerpb.ResourceUsage)
	GetRoomState func() *peerpb.RoomState
}

// NewPeerGRPCServer creates a new peer-to-peer gRPC server.
func NewPeerGRPCServer(cfg PeerGRPCServerConfig) (*PeerGRPCServer, error) {
	compressor, err := NewTensorCompressor()
	if err != nil {
		return nil, fmt.Errorf("failed to create tensor compressor: %w", err)
	}

	return &PeerGRPCServer{
		localPeerID:  cfg.LocalPeerID,
		localWorker:  cfg.LocalWorker,
		roomToken:    cfg.RoomToken,
		compressor:   compressor,
		onHandshake:  cfg.OnHandshake,
		getRoomState: cfg.GetRoomState,
	}, nil
}

// Start begins listening for peer gRPC requests on the given port.
func (s *PeerGRPCServer) Start(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	s.server = grpc.NewServer(
		grpc.MaxRecvMsgSize(256*1024*1024), // 256MB for large tensors
		grpc.MaxSendMsgSize(256*1024*1024),
	)
	peerpb.RegisterPeerServiceServer(s.server, s)

	logger.Info("peer gRPC server starting", "port", port)

	go func() {
		if err := s.server.Serve(lis); err != nil {
			logger.Error("peer gRPC server stopped", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the peer gRPC server.
func (s *PeerGRPCServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
	if s.compressor != nil {
		s.compressor.Close()
	}
	logger.Info("peer gRPC server stopped")
}

// Handshake handles incoming peer handshake requests.
func (s *PeerGRPCServer) Handshake(ctx context.Context, req *peerpb.HandshakeRequest) (*peerpb.HandshakeResponse, error) {
	logger.Info("handshake received",
		"from_peer", req.PeerId,
		"has_resources", req.Resources != nil,
	)

	// Validate room token
	s.mu.RLock()
	expectedToken := s.roomToken
	s.mu.RUnlock()

	if expectedToken != "" && req.RoomToken != expectedToken {
		return &peerpb.HandshakeResponse{
			Accepted: false,
			Error:    "invalid room token",
		}, nil
	}

	// Notify handler of new peer
	if s.onHandshake != nil {
		s.onHandshake(req.PeerId, req.Resources)
	}

	// Get current room state
	var roomState *peerpb.RoomState
	if s.getRoomState != nil {
		roomState = s.getRoomState()
	}

	return &peerpb.HandshakeResponse{
		Accepted:  true,
		RoomState: roomState,
	}, nil
}

// SyncState handles state synchronization requests from peers.
func (s *PeerGRPCServer) SyncState(ctx context.Context, req *peerpb.SyncRequest) (*peerpb.SyncResponse, error) {
	s.mu.RLock()
	currentVersion := s.stateVersion
	s.mu.RUnlock()

	// If peer is already up to date, return current version
	if req.StateVersion >= currentVersion {
		return &peerpb.SyncResponse{
			StateVersion: currentVersion,
		}, nil
	}

	var roomState *peerpb.RoomState
	if s.getRoomState != nil {
		roomState = s.getRoomState()
	}

	return &peerpb.SyncResponse{
		RoomState:    roomState,
		StateVersion: currentVersion,
	}, nil
}

// HealthCheck responds to peer health check pings.
func (s *PeerGRPCServer) HealthCheck(ctx context.Context, req *peerpb.Ping) (*peerpb.Pong, error) {
	now := time.Now().UnixMilli()
	latency := float32(now - req.Timestamp)

	return &peerpb.Pong{
		Timestamp: now,
		LatencyMs: latency,
	}, nil
}

// ForwardTensor receives a tensor from a peer, processes it through local layers,
// and returns the result.
func (s *PeerGRPCServer) ForwardTensor(ctx context.Context, req *workerpb.ForwardRequest) (*workerpb.ForwardResponse, error) {
	start := time.Now()

	logger.Info("forward tensor received",
		"request_id", req.RequestId,
		"from_layer", req.Meta.GetFromLayer(),
		"to_layer", req.Meta.GetToLayer(),
		"input_size", len(req.TensorData),
		"compressed", req.Compressed,
	)

	// Decompress if needed
	tensorData := req.TensorData
	if req.Compressed && s.compressor != nil {
		var err error
		tensorData, err = s.compressor.Decompress(tensorData)
		if err != nil {
			return nil, fmt.Errorf("tensor decompression failed: %w", err)
		}

		logger.Info("tensor decompressed",
			"compressed_size", len(req.TensorData),
			"decompressed_size", len(tensorData),
		)
	}

	// Forward to local Python worker
	client := s.localWorker()
	if client == nil {
		return nil, fmt.Errorf("local worker unavailable")
	}

	localReq := &workerpb.ForwardRequest{
		RequestId:  req.RequestId,
		TensorData: tensorData,
		Meta:       req.Meta,
		Compressed: false, // Already decompressed
		Checksum:   req.Checksum,
	}

	resp, err := client.ForwardPass(ctx, localReq)
	if err != nil {
		return nil, fmt.Errorf("local forward pass failed: %w", err)
	}

	// Compress output if beneficial
	outputData := resp.TensorData
	compressed := false
	if s.compressor != nil {
		var ratio float64
		outputData, compressed, ratio = s.compressor.CompressIfBeneficial(resp.TensorData)
		if compressed {
			logger.Info("output tensor compressed",
				"original_size", len(resp.TensorData),
				"compressed_size", len(outputData),
				"ratio", fmt.Sprintf("%.2f", ratio),
			)
		}
	}

	duration := float32(time.Since(start).Milliseconds())

	return &workerpb.ForwardResponse{
		RequestId:  req.RequestId,
		TensorData: outputData,
		Meta:       resp.Meta,
		Compressed: compressed,
		Checksum:   resp.Checksum,
		DurationMs: duration,
	}, nil
}

// ForwardTensorStream handles streaming tensor forwarding for large models.
func (s *PeerGRPCServer) ForwardTensorStream(stream peerpb.PeerService_ForwardTensorStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		resp, err := s.ForwardTensor(stream.Context(), req)
		if err != nil {
			return err
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// EmbedTokens forwards an embed tokens request to the local Python worker.
func (s *PeerGRPCServer) EmbedTokens(ctx context.Context, req *workerpb.EmbedRequest) (*workerpb.EmbedResponse, error) {
	logger.Info("embed tokens received",
		"request_id", req.RequestId,
		"has_text", req.Text != "",
		"num_token_ids", len(req.TokenIds),
	)

	client := s.localWorker()
	if client == nil {
		return nil, fmt.Errorf("local worker unavailable")
	}

	resp, err := client.EmbedTokens(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("local embed tokens failed: %w", err)
	}

	return resp, nil
}

// SampleTokens forwards a sample tokens request to the local Python worker.
func (s *PeerGRPCServer) SampleTokens(ctx context.Context, req *workerpb.SampleRequest) (*workerpb.SampleResponse, error) {
	logger.Info("sample tokens received",
		"request_id", req.RequestId,
		"temperature", req.Temperature,
	)

	client := s.localWorker()
	if client == nil {
		return nil, fmt.Errorf("local worker unavailable")
	}

	resp, err := client.SampleTokens(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("local sample tokens failed: %w", err)
	}

	return resp, nil
}

// SetRoomToken updates the room token for peer authentication.
func (s *PeerGRPCServer) SetRoomToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roomToken = token
}

// IncrementStateVersion bumps the state version for sync tracking.
func (s *PeerGRPCServer) IncrementStateVersion() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateVersion++
	return s.stateVersion
}
