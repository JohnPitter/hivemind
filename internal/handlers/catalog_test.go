package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaopedro/hivemind/internal/catalog"
	"github.com/joaopedro/hivemind/internal/handlers"
)

func TestListCatalog(t *testing.T) {
	h := handlers.NewCatalogHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models/catalog", nil)
	rec := httptest.NewRecorder()

	h.ListCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Models    []catalog.ModelRequirements `json:"models"`
		Suggested *catalog.ModelRequirements  `json:"suggested"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(resp.Models) != 20 {
		t.Errorf("expected 20 models, got %d", len(resp.Models))
	}

	if resp.Suggested != nil {
		t.Error("expected no suggestion without vram_mb param")
	}
}

func TestListCatalog_WithVRAM(t *testing.T) {
	h := handlers.NewCatalogHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models/catalog?vram_mb=10000", nil)
	rec := httptest.NewRecorder()

	h.ListCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Models    []catalog.ModelRequirements `json:"models"`
		Suggested *catalog.ModelRequirements  `json:"suggested"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Suggested == nil {
		t.Fatal("expected a suggestion with vram_mb=10000")
	}

	if resp.Suggested.ID != "stabilityai/stable-diffusion-xl" {
		t.Errorf("expected SDXL suggestion for 10000MB, got %s", resp.Suggested.ID)
	}
}

func TestListCatalog_InvalidVRAM(t *testing.T) {
	h := handlers.NewCatalogHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models/catalog?vram_mb=abc", nil)
	rec := httptest.NewRecorder()

	h.ListCatalog(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
