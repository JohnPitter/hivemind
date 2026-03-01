package models

// ResourceCheckResult describes whether a room has enough resources for a model.
type ResourceCheckResult struct {
	Sufficient       bool   `json:"sufficient"`
	TotalVRAMMB      int64  `json:"total_vram_mb"`
	RequiredVRAMMB   int64  `json:"required_vram_mb"`
	DeficitMB        int64  `json:"deficit_mb,omitempty"`
	SuggestedModelID string `json:"suggested_model_id,omitempty"`
	SuggestedModel   string `json:"suggested_model_name,omitempty"`
	PeerCount        int    `json:"peer_count"`
}

// CreateRoomResponse wraps room creation with resource validation info.
type CreateRoomResponse struct {
	Room           *Room                `json:"room"`
	ResourceCheck  *ResourceCheckResult `json:"resource_check,omitempty"`
	PendingTimeout int                  `json:"pending_timeout_seconds,omitempty"`
}
