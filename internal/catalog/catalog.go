package catalog

import "github.com/joaopedro/hivemind/internal/models"

// ModelRequirements describes the compute requirements for a model.
type ModelRequirements struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Type            models.ModelType `json:"type"`
	ParameterSize   string           `json:"parameter_size"`
	TotalLayers     int              `json:"total_layers"`
	MinVRAMMB       int64            `json:"min_vram_mb"`
	RecommendedVRAM int64            `json:"recommended_vram_mb"`
	MinPeerVRAMMB   int64            `json:"min_peer_vram_mb"`
}

// catalog is the sorted list of known models (descending by MinVRAMMB).
var catalog = []ModelRequirements{
	// Code — 131072 MB
	{
		ID:              "deepseek-ai/DeepSeek-Coder-V2-236B",
		Name:            "DeepSeek Coder V2 236B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "236B",
		TotalLayers:     96,
		MinVRAMMB:       131072,
		RecommendedVRAM: 163840,
		MinPeerVRAMMB:   8192,
	},
	// Multimodal — 45056 MB
	{
		ID:              "Qwen/Qwen2-VL-72B",
		Name:            "Qwen2-VL 72B",
		Type:            models.ModelTypeMultimodal,
		ParameterSize:   "72B",
		TotalLayers:     80,
		MinVRAMMB:       45056,
		RecommendedVRAM: 53248,
		MinPeerVRAMMB:   4096,
	},
	// LLM — 40960 MB
	{
		ID:              "meta-llama/Llama-3-70B",
		Name:            "Llama 3 70B",
		Type:            models.ModelTypeLLM,
		ParameterSize:   "70B",
		TotalLayers:     80,
		MinVRAMMB:       40960,
		RecommendedVRAM: 49152,
		MinPeerVRAMMB:   4096,
	},
	// LLM — 28672 MB
	{
		ID:              "mistralai/Mixtral-8x7B",
		Name:            "Mixtral 8x7B",
		Type:            models.ModelTypeLLM,
		ParameterSize:   "47B",
		TotalLayers:     32,
		MinVRAMMB:       28672,
		RecommendedVRAM: 32768,
		MinPeerVRAMMB:   4096,
	},
	// Code — 20480 MB
	{
		ID:              "Qwen/Qwen2.5-Coder-32B",
		Name:            "Qwen2.5-Coder 32B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "32B",
		TotalLayers:     64,
		MinVRAMMB:       20480,
		RecommendedVRAM: 24576,
		MinPeerVRAMMB:   4096,
	},
	// Code — 20480 MB
	{
		ID:              "codellama/CodeLlama-34B",
		Name:            "CodeLlama 34B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "34B",
		TotalLayers:     48,
		MinVRAMMB:       20480,
		RecommendedVRAM: 24576,
		MinPeerVRAMMB:   4096,
	},
	// Code — 20480 MB
	{
		ID:              "deepseek-ai/DeepSeek-Coder-33B",
		Name:            "DeepSeek Coder 33B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "33B",
		TotalLayers:     48,
		MinVRAMMB:       20480,
		RecommendedVRAM: 24576,
		MinPeerVRAMMB:   4096,
	},
	// Multimodal — 20480 MB
	{
		ID:              "liuhaotian/LLaVA-34B",
		Name:            "LLaVA 34B",
		Type:            models.ModelTypeMultimodal,
		ParameterSize:   "34B",
		TotalLayers:     48,
		MinVRAMMB:       20480,
		RecommendedVRAM: 24576,
		MinPeerVRAMMB:   4096,
	},
	// LLM — 18432 MB
	{
		ID:              "google/gemma-2-27b",
		Name:            "Gemma 2 27B",
		Type:            models.ModelTypeLLM,
		ParameterSize:   "27B",
		TotalLayers:     32,
		MinVRAMMB:       18432,
		RecommendedVRAM: 24576,
		MinPeerVRAMMB:   4096,
	},
	// Diffusion — 12288 MB
	{
		ID:              "black-forest-labs/FLUX.1-dev",
		Name:            "FLUX.1 Dev",
		Type:            models.ModelTypeDiffusion,
		ParameterSize:   "12B",
		TotalLayers:     24,
		MinVRAMMB:       12288,
		RecommendedVRAM: 16384,
		MinPeerVRAMMB:   4096,
	},
	// Code — 10240 MB
	{
		ID:              "bigcode/StarCoder2-15B",
		Name:            "StarCoder2 15B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "15B",
		TotalLayers:     40,
		MinVRAMMB:       10240,
		RecommendedVRAM: 12288,
		MinPeerVRAMMB:   2048,
	},
	// Diffusion — 8192 MB
	{
		ID:              "stabilityai/stable-diffusion-xl",
		Name:            "Stable Diffusion XL",
		Type:            models.ModelTypeDiffusion,
		ParameterSize:   "3.5B",
		TotalLayers:     24,
		MinVRAMMB:       8192,
		RecommendedVRAM: 10240,
		MinPeerVRAMMB:   2048,
	},
	// Code — 8192 MB
	{
		ID:              "codellama/CodeLlama-13B",
		Name:            "CodeLlama 13B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "13B",
		TotalLayers:     40,
		MinVRAMMB:       8192,
		RecommendedVRAM: 10240,
		MinPeerVRAMMB:   2048,
	},
	// Multimodal — 8192 MB
	{
		ID:              "liuhaotian/LLaVA-13B",
		Name:            "LLaVA 13B",
		Type:            models.ModelTypeMultimodal,
		ParameterSize:   "13B",
		TotalLayers:     40,
		MinVRAMMB:       8192,
		RecommendedVRAM: 10240,
		MinPeerVRAMMB:   2048,
	},
	// LLM — 6144 MB
	{
		ID:              "meta-llama/Llama-3-8B",
		Name:            "Llama 3 8B",
		Type:            models.ModelTypeLLM,
		ParameterSize:   "8B",
		TotalLayers:     32,
		MinVRAMMB:       6144,
		RecommendedVRAM: 8192,
		MinPeerVRAMMB:   2048,
	},
	// Code — 5120 MB
	{
		ID:              "Qwen/Qwen2.5-Coder-7B",
		Name:            "Qwen2.5-Coder 7B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "7B",
		TotalLayers:     32,
		MinVRAMMB:       5120,
		RecommendedVRAM: 6144,
		MinPeerVRAMMB:   2048,
	},
	// LLM — 2048 MB
	{
		ID:              "TinyLlama/TinyLlama-1.1B",
		Name:            "TinyLlama 1.1B",
		Type:            models.ModelTypeLLM,
		ParameterSize:   "1.1B",
		TotalLayers:     22,
		MinVRAMMB:       2048,
		RecommendedVRAM: 4096,
		MinPeerVRAMMB:   1024,
	},
	// Code — 2048 MB
	{
		ID:              "bigcode/StarCoder2-3B",
		Name:            "StarCoder2 3B",
		Type:            models.ModelTypeCode,
		ParameterSize:   "3B",
		TotalLayers:     26,
		MinVRAMMB:       2048,
		RecommendedVRAM: 3072,
		MinPeerVRAMMB:   1024,
	},
	// Embedding — 1024 MB
	{
		ID:              "BAAI/bge-large-en-v1.5",
		Name:            "BGE Large",
		Type:            models.ModelTypeEmbedding,
		ParameterSize:   "335M",
		TotalLayers:     24,
		MinVRAMMB:       1024,
		RecommendedVRAM: 2048,
		MinPeerVRAMMB:   512,
	},
	// Embedding — 512 MB
	{
		ID:              "nomic-ai/nomic-embed-text-v1.5",
		Name:            "Nomic Embed Text",
		Type:            models.ModelTypeEmbedding,
		ParameterSize:   "137M",
		TotalLayers:     12,
		MinVRAMMB:       512,
		RecommendedVRAM: 1024,
		MinPeerVRAMMB:   256,
	},
}

// index provides O(1) lookup by model ID.
var index map[string]*ModelRequirements

func init() {
	index = make(map[string]*ModelRequirements, len(catalog))
	for i := range catalog {
		index[catalog[i].ID] = &catalog[i]
	}
}

// All returns the full catalog sorted descending by MinVRAMMB.
func All() []ModelRequirements {
	out := make([]ModelRequirements, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns a model's requirements or nil if not in catalog.
func Lookup(modelID string) *ModelRequirements {
	return index[modelID]
}

// IsKnown returns true if the model exists in the catalog.
func IsKnown(modelID string) bool {
	_, ok := index[modelID]
	return ok
}

// LayersForModel returns the layer count for a known model, or 32 as default.
func LayersForModel(modelID string) int {
	if m := index[modelID]; m != nil {
		return m.TotalLayers
	}
	return 32
}

// SuggestLargestFitting returns the biggest model that fits in the given VRAM.
// Returns nil if no model fits.
func SuggestLargestFitting(availableVRAMMB int64) *ModelRequirements {
	for i := range catalog {
		if catalog[i].MinVRAMMB <= availableVRAMMB {
			return &catalog[i]
		}
	}
	return nil
}

// SuggestForResources returns the biggest model that fits the peer's usable VRAM.
func SuggestForResources(res models.ResourceSpec) *ModelRequirements {
	return SuggestLargestFitting(res.TotalUsableVRAM())
}

// FilterByType returns all models of the given type.
func FilterByType(modelType models.ModelType) []ModelRequirements {
	var out []ModelRequirements
	for _, m := range catalog {
		if m.Type == modelType {
			out = append(out, m)
		}
	}
	return out
}
