package models

// ResourceSpec describes the compute resources a peer contributes.
type ResourceSpec struct {
	GPUName    string `json:"gpu_name"`
	VRAMTotal  int64  `json:"vram_total_mb"`
	VRAMFree   int64  `json:"vram_free_mb"`
	RAMTotal   int64  `json:"ram_total_mb"`
	RAMFree    int64  `json:"ram_free_mb"`
	CUDAAvail  bool   `json:"cuda_available"`
	Platform   string `json:"platform"`
}

// CanContribute checks if the peer has enough resources to participate.
func (r ResourceSpec) CanContribute(minVRAM int64) bool {
	return r.VRAMFree >= minVRAM
}

// TotalUsableVRAM returns the VRAM available for model layers.
func (r ResourceSpec) TotalUsableVRAM() int64 {
	// Reserve 512MB for overhead
	const reserveMB int64 = 512
	if r.VRAMFree <= reserveMB {
		return 0
	}
	return r.VRAMFree - reserveMB
}
