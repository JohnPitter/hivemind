package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/joaopedro/hivemind/internal/models"
)

// generateID creates a random hex string of n bytes (2n hex chars).
func generateID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// assignLayers distributes model layers across peers by available VRAM.
func assignLayers(room *models.Room) {
	if len(room.Peers) == 0 || room.TotalLayers == 0 {
		return
	}

	var totalVRAM int64
	for _, p := range room.Peers {
		totalVRAM += p.Resources.TotalUsableVRAM()
	}

	if totalVRAM == 0 {
		// Equal distribution fallback
		perPeer := room.TotalLayers / len(room.Peers)
		offset := 0
		for i := range room.Peers {
			count := perPeer
			if i == len(room.Peers)-1 {
				count = room.TotalLayers - offset
			}
			room.Peers[i].Layers = makeRange(offset, offset+count)
			offset += count
		}
		return
	}

	// Proportional distribution by VRAM
	offset := 0
	for i := range room.Peers {
		peerVRAM := room.Peers[i].Resources.TotalUsableVRAM()
		proportion := float64(peerVRAM) / float64(totalVRAM)
		count := int(proportion * float64(room.TotalLayers))

		if count < 1 {
			count = 1
		}
		if i == len(room.Peers)-1 {
			count = room.TotalLayers - offset
		}
		if offset+count > room.TotalLayers {
			count = room.TotalLayers - offset
		}

		room.Peers[i].Layers = makeRange(offset, offset+count)
		offset += count
	}
}

// makeRange creates a slice of consecutive ints [start, end).
func makeRange(start, end int) []int {
	r := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		r = append(r, i)
	}
	return r
}

// parseSize parses "1024x1024" into width and height.
func parseSize(size string) (int, int) {
	var w, h int
	if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil {
		return 1024, 1024
	}
	return w, h
}
