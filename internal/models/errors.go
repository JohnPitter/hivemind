package models

import "errors"

// Sentinel errors for clean error handling across layers.
var (
	// Room errors
	ErrRoomNotFound     = errors.New("room not found")
	ErrRoomFull         = errors.New("room is full")
	ErrRoomClosed       = errors.New("room is closed")
	ErrInvalidInvite    = errors.New("invalid invite code")
	ErrAlreadyInRoom    = errors.New("already in a room")
	ErrNotInRoom        = errors.New("not in any room")
	ErrNotHost          = errors.New("only the host can perform this action")

	// Peer errors
	ErrPeerNotFound     = errors.New("peer not found")
	ErrPeerOffline      = errors.New("peer is offline")
	ErrInsufficientVRAM = errors.New("insufficient VRAM to participate")

	// Inference errors
	ErrModelNotLoaded   = errors.New("model not loaded")
	ErrInferenceTimeout = errors.New("inference request timed out")
	ErrWorkerUnavail    = errors.New("worker is unavailable")

	// Network errors
	ErrMeshNotReady     = errors.New("wireguard mesh not ready")
	ErrSignalingFailed  = errors.New("signaling server connection failed")
)
