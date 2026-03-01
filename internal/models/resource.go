package models

// ResourceSpec describes the compute resources a peer contributes.
type ResourceSpec struct {
	GPUName     string `json:"gpu_name"`
	VRAMTotal   int64  `json:"vram_total_mb"`
	VRAMFree    int64  `json:"vram_free_mb"`
	RAMTotal    int64  `json:"ram_total_mb"`
	RAMFree     int64  `json:"ram_free_mb"`
	CUDAAvail   bool   `json:"cuda_available"`
	Platform    string `json:"platform"`
	DonationPct int    `json:"donation_pct,omitempty"`
}

// effectiveDonationPct returns the donation percentage, defaulting to 100
// when DonationPct is zero (backwards compatibility).
func (r ResourceSpec) effectiveDonationPct() int {
	if r.DonationPct <= 0 {
		return 100
	}
	return r.DonationPct
}

// CanContribute checks if the peer has enough resources to participate.
func (r ResourceSpec) CanContribute(minVRAM int64) bool {
	return r.VRAMFree >= minVRAM
}

// TotalUsableVRAM returns the VRAM available for model layers,
// scaled by the donation percentage.
func (r ResourceSpec) TotalUsableVRAM() int64 {
	const reserveMB int64 = 512
	if r.VRAMFree <= reserveMB {
		return 0
	}
	free := r.VRAMFree - reserveMB
	return free * int64(r.effectiveDonationPct()) / 100
}

// TotalUsableRAM returns the RAM available for model offloading,
// scaled by the donation percentage.
func (r ResourceSpec) TotalUsableRAM() int64 {
	const reserveMB int64 = 1024
	if r.RAMFree <= reserveMB {
		return 0
	}
	free := r.RAMFree - reserveMB
	return free * int64(r.effectiveDonationPct()) / 100
}
