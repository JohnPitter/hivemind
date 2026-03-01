package catalog

import (
	"testing"

	"github.com/joaopedro/hivemind/internal/models"
)

func TestLookup_KnownModel(t *testing.T) {
	m := Lookup("meta-llama/Llama-3-70B")
	if m == nil {
		t.Fatal("expected non-nil result for known model")
	}
	if m.TotalLayers != 80 {
		t.Errorf("expected 80 layers, got %d", m.TotalLayers)
	}
	if m.MinVRAMMB != 40960 {
		t.Errorf("expected 40960 MinVRAMMB, got %d", m.MinVRAMMB)
	}
}

func TestLookup_UnknownModel(t *testing.T) {
	m := Lookup("unknown/model")
	if m != nil {
		t.Error("expected nil for unknown model")
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("meta-llama/Llama-3-8B") {
		t.Error("expected Llama 3 8B to be known")
	}
	if IsKnown("nonexistent") {
		t.Error("expected nonexistent model to be unknown")
	}
}

func TestLayersForModel_Known(t *testing.T) {
	layers := LayersForModel("meta-llama/Llama-3-70B")
	if layers != 80 {
		t.Errorf("expected 80 layers, got %d", layers)
	}
}

func TestLayersForModel_Unknown(t *testing.T) {
	layers := LayersForModel("unknown/model")
	if layers != 32 {
		t.Errorf("expected default 32 layers, got %d", layers)
	}
}

func TestSuggestLargestFitting(t *testing.T) {
	tests := []struct {
		vram    int64
		wantID  string
		wantNil bool
	}{
		{200000, "deepseek-ai/DeepSeek-Coder-V2-236B", false},
		{131072, "deepseek-ai/DeepSeek-Coder-V2-236B", false},
		{50000, "Qwen/Qwen2-VL-72B", false},
		{45056, "Qwen/Qwen2-VL-72B", false},
		{40960, "meta-llama/Llama-3-70B", false},
		{30000, "mistralai/Mixtral-8x7B", false},
		{10000, "stabilityai/stable-diffusion-xl", false},
		{6144, "meta-llama/Llama-3-8B", false},
		{2048, "TinyLlama/TinyLlama-1.1B", false},
		{1024, "BAAI/bge-large-en-v1.5", false},
		{512, "nomic-ai/nomic-embed-text-v1.5", false},
		{400, "", true},
	}

	for _, tt := range tests {
		m := SuggestLargestFitting(tt.vram)
		if tt.wantNil {
			if m != nil {
				t.Errorf("vram=%d: expected nil, got %s", tt.vram, m.ID)
			}
			continue
		}
		if m == nil {
			t.Errorf("vram=%d: expected %s, got nil", tt.vram, tt.wantID)
			continue
		}
		if m.ID != tt.wantID {
			t.Errorf("vram=%d: expected %s, got %s", tt.vram, tt.wantID, m.ID)
		}
	}
}

func TestCatalogCount(t *testing.T) {
	all := All()
	if len(all) != 20 {
		t.Errorf("expected 20 models in catalog, got %d", len(all))
	}
}

func TestCatalogSortedDescending(t *testing.T) {
	all := All()
	for i := 1; i < len(all); i++ {
		if all[i].MinVRAMMB > all[i-1].MinVRAMMB {
			t.Errorf("catalog not sorted descending: %s (%d MB) > %s (%d MB)",
				all[i].ID, all[i].MinVRAMMB, all[i-1].ID, all[i-1].MinVRAMMB)
		}
	}
}

func TestFilterByType(t *testing.T) {
	tests := []struct {
		modelType models.ModelType
		wantCount int
	}{
		{models.ModelTypeLLM, 5},
		{models.ModelTypeCode, 8},
		{models.ModelTypeDiffusion, 2},
		{models.ModelTypeMultimodal, 3},
		{models.ModelTypeEmbedding, 2},
	}

	for _, tt := range tests {
		got := FilterByType(tt.modelType)
		if len(got) != tt.wantCount {
			t.Errorf("FilterByType(%s): expected %d models, got %d", tt.modelType, tt.wantCount, len(got))
		}
	}
}

func TestAllReturnsCopy(t *testing.T) {
	a := All()
	a[0].Name = "MODIFIED"

	b := All()
	if b[0].Name == "MODIFIED" {
		t.Error("All() should return a copy, not a reference to internal data")
	}
}
