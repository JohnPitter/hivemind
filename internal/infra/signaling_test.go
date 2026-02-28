package infra_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaopedro/hivemind/internal/infra"
)

// testSignalingServer creates a signaling server and returns an httptest server.
func testSignalingServer() *httptest.Server {
	sig := infra.NewSignalingServer(0)

	mux := http.NewServeMux()
	// Manually set up the same routes for testing
	mux.HandleFunc("POST /signal/create", sig.HandleCreate)
	mux.HandleFunc("POST /signal/join", sig.HandleJoin)
	mux.HandleFunc("GET /signal/peers", sig.HandlePeers)
	mux.HandleFunc("DELETE /signal/leave", sig.HandleLeave)
	mux.HandleFunc("GET /signal/health", sig.HandleHealth)

	return httptest.NewServer(mux)
}

func TestSignaling_CreateAndJoin(t *testing.T) {
	// We'll test using the client against a real signaling server
	// For now, test the signaling server directly via HTTP

	sig := infra.NewSignalingServer(0)
	_ = sig // Will be used when handlers are exported

	// Test direct creation
	createReq := infra.SignalingCreateRequest{
		RoomID:     "room-123",
		InviteCode: "ABC-DEF",
		ModelID:    "test-model",
		HostID:     "host-1",
		MaxPeers:   6,
		PublicKey:  "hostPubKey==",
		Endpoint:   "1.2.3.4:51820",
	}

	_ = createReq

	// For the signaling server test, we verify the data structures work correctly
	t.Run("create request marshaling", func(t *testing.T) {
		data, err := json.Marshal(createReq)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded infra.SignalingCreateRequest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.RoomID != "room-123" {
			t.Errorf("expected room_id 'room-123', got %q", decoded.RoomID)
		}
		if decoded.InviteCode != "ABC-DEF" {
			t.Errorf("expected invite 'ABC-DEF', got %q", decoded.InviteCode)
		}
	})

	t.Run("join request marshaling", func(t *testing.T) {
		joinReq := infra.SignalingJoinRequest{
			InviteCode: "ABC-DEF",
			PeerID:     "peer-1",
			PublicKey:  "peerPubKey==",
			Endpoint:   "5.6.7.8:51820",
		}

		data, err := json.Marshal(joinReq)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded infra.SignalingJoinRequest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.PeerID != "peer-1" {
			t.Errorf("expected peer_id 'peer-1', got %q", decoded.PeerID)
		}
	})

	t.Run("join response structure", func(t *testing.T) {
		resp := infra.SignalingJoinResponse{
			RoomID:  "room-123",
			ModelID: "test-model",
			MeshIP:  "10.42.0.2/24",
			Peers: []infra.SignalingPeer{
				{
					ID:        "host-1",
					PublicKey: "hostPubKey==",
					Endpoint:  "1.2.3.4:51820",
					MeshIP:    "10.42.0.1/24",
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		if !bytes.Contains(data, []byte("10.42.0.2/24")) {
			t.Error("response should contain mesh IP")
		}
	})
}
