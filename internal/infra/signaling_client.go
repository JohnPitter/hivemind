package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/joaopedro/hivemind/internal/logger"
)

// SignalingClient connects to the signaling server for room discovery and key exchange.
type SignalingClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSignalingClient creates a client for the signaling server.
func NewSignalingClient(signalingURL string) *SignalingClient {
	return &SignalingClient{
		baseURL: signalingURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CreateRoom registers a new room on the signaling server.
func (c *SignalingClient) CreateRoom(ctx context.Context, req SignalingCreateRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/signal/create", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("signaling server unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("signaling error: %s (status %d)", errResp["error"], resp.StatusCode)
	}

	logger.Info("room registered with signaling server", "room_id", req.RoomID)
	return nil
}

// JoinRoom sends a join request to the signaling server, receives peer list for WireGuard config.
func (c *SignalingClient) JoinRoom(ctx context.Context, req SignalingJoinRequest) (*SignalingJoinResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/signal/join", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("signaling server unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("signaling error: %s (status %d)", errResp["error"], resp.StatusCode)
	}

	var joinResp SignalingJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&joinResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Info("joined room via signaling",
		"room_id", joinResp.RoomID,
		"mesh_ip", joinResp.MeshIP,
		"peers", len(joinResp.Peers),
	)

	return &joinResp, nil
}

// LeaveRoom notifies the signaling server that this peer is leaving.
func (c *SignalingClient) LeaveRoom(ctx context.Context, inviteCode, peerID string) error {
	url := fmt.Sprintf("%s/signal/leave?invite=%s&peer_id=%s", c.baseURL, inviteCode, peerID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("signaling server unreachable: %w", err)
	}
	defer resp.Body.Close()

	logger.Info("left room via signaling", "invite", inviteCode, "peer_id", peerID)
	return nil
}

// GetPeers retrieves the current peer list for a room.
func (c *SignalingClient) GetPeers(ctx context.Context, inviteCode string) ([]SignalingPeer, error) {
	url := fmt.Sprintf("%s/signal/peers?invite=%s", c.baseURL, inviteCode)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("signaling server unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signaling error: status %d", resp.StatusCode)
	}

	var peers []SignalingPeer
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, fmt.Errorf("failed to decode peers: %w", err)
	}

	return peers, nil
}
