package infra

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joaopedro/hivemind/internal/logger"
)

// GetOrCreatePeerID returns a persistent peer ID, creating one if it doesn't exist.
// The ID is stored at ~/.hivemind/peer_id.
func GetOrCreatePeerID() string {
	// Check env override first
	if id := os.Getenv("HIVEMIND_PEER_ID"); id != "" {
		return id
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return generatePeerID()
	}

	dir := filepath.Join(home, ".hivemind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return generatePeerID()
	}

	path := filepath.Join(dir, "peer_id")

	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id
		}
	}

	id := generatePeerID()
	if err := os.WriteFile(path, []byte(id), 0o644); err != nil {
		logger.Warn("failed to persist peer ID", "error", err)
	}

	logger.Info("generated new peer ID", "id", id)
	return id
}

func generatePeerID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("peer-%d", os.Getpid())
	}
	return "peer-" + hex.EncodeToString(b)
}
