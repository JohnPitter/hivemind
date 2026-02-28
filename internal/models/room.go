package models

import "time"

// RoomState represents the lifecycle state of a room.
type RoomState string

const (
	RoomStateCreating RoomState = "creating"
	RoomStateActive   RoomState = "active"
	RoomStatePaused   RoomState = "paused"
	RoomStateClosed   RoomState = "closed"
)

// Room represents a cooperative inference room.
type Room struct {
	ID          string    `json:"id"`
	InviteCode  string    `json:"invite_code"`
	ModelID     string    `json:"model_id"`
	ModelType   ModelType `json:"model_type"`
	State       RoomState `json:"state"`
	HostID      string    `json:"host_id"`
	MaxPeers    int       `json:"max_peers"`
	TotalLayers int       `json:"total_layers"`
	CreatedAt   time.Time `json:"created_at"`
	Peers       []Peer    `json:"peers"`
}

// ModelType distinguishes between LLM and diffusion models.
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeDiffusion ModelType = "diffusion"
)

// PeerState represents a peer's connection state.
type PeerState string

const (
	PeerStateConnecting PeerState = "connecting"
	PeerStateSyncing    PeerState = "syncing"
	PeerStateReady      PeerState = "ready"
	PeerStateOffline    PeerState = "offline"
)

// Peer represents a node in the room.
type Peer struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	IP        string       `json:"ip"`
	State     PeerState    `json:"state"`
	Layers    []int        `json:"layers"`
	Resources ResourceSpec `json:"resources"`
	Latency   float64      `json:"latency_ms"`
	JoinedAt  time.Time    `json:"joined_at"`
	IsHost    bool         `json:"is_host"`
}

// RoomConfig holds settings for creating a new room.
type RoomConfig struct {
	ModelID     string    `json:"model_id"`
	ModelType   ModelType `json:"model_type"`
	MaxPeers    int       `json:"max_peers"`
	AutoApprove bool      `json:"auto_approve"`
}

// RoomStatus holds real-time room status for API/CLI display.
type RoomStatus struct {
	Room         Room    `json:"room"`
	TotalVRAM    int64   `json:"total_vram_mb"`
	UsedVRAM     int64   `json:"used_vram_mb"`
	TokensPerSec float64 `json:"tokens_per_sec"`
	Uptime       string  `json:"uptime"`
}
