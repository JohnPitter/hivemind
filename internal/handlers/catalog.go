package handlers

import (
	"net/http"
	"strconv"

	"github.com/joaopedro/hivemind/internal/catalog"
)

// CatalogHandler handles model catalog endpoints.
type CatalogHandler struct{}

// NewCatalogHandler creates a catalog handler.
func NewCatalogHandler() *CatalogHandler {
	return &CatalogHandler{}
}

// catalogResponse is the API response for the catalog endpoint.
type catalogResponse struct {
	Models    []catalog.ModelRequirements `json:"models"`
	Suggested *catalog.ModelRequirements  `json:"suggested,omitempty"`
}

// ListCatalog handles GET /v1/models/catalog.
// Optional query param: vram_mb=<int> to get a suggested model.
func (h *CatalogHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	resp := catalogResponse{
		Models: catalog.All(),
	}

	if vramStr := r.URL.Query().Get("vram_mb"); vramStr != "" {
		vram, err := strconv.ParseInt(vramStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "vram_mb must be an integer")
			return
		}
		resp.Suggested = catalog.SuggestLargestFitting(vram)
	}

	writeJSON(w, http.StatusOK, resp)
}
