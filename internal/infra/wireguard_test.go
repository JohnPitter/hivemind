package infra_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joaopedro/hivemind/internal/infra"
)

func TestGenerateKeyPair(t *testing.T) {
	priv, pub, err := infra.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	// Keys should be base64 encoded 32-byte values
	privBytes, err := base64.StdEncoding.DecodeString(priv)
	if err != nil {
		t.Fatalf("private key not valid base64: %v", err)
	}
	if len(privBytes) != 32 {
		t.Errorf("private key should be 32 bytes, got %d", len(privBytes))
	}

	pubBytes, err := base64.StdEncoding.DecodeString(pub)
	if err != nil {
		t.Fatalf("public key not valid base64: %v", err)
	}
	if len(pubBytes) != 32 {
		t.Errorf("public key should be 32 bytes, got %d", len(pubBytes))
	}

	// Keys should be different
	if priv == pub {
		t.Error("private and public keys should be different")
	}
}

func TestGenerateKeyPair_Unique(t *testing.T) {
	_, pub1, _ := infra.GenerateKeyPair()
	_, pub2, _ := infra.GenerateKeyPair()

	if pub1 == pub2 {
		t.Error("two generated keypairs should have different public keys")
	}
}

func TestWireGuardManager_Initialize(t *testing.T) {
	dir := t.TempDir()
	wg := infra.NewWireGuardManager(dir)

	err := wg.Initialize(51820, "10.42.0.1/24")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	if wg.PublicKey() == "" {
		t.Error("public key should be set after initialization")
	}

	if wg.MeshAddress() != "10.42.0.1/24" {
		t.Errorf("expected mesh address '10.42.0.1/24', got %q", wg.MeshAddress())
	}
}

func TestWireGuardManager_AddRemovePeer(t *testing.T) {
	dir := t.TempDir()
	wg := infra.NewWireGuardManager(dir)
	wg.Initialize(51820, "10.42.0.1/24")

	err := wg.AddPeer("peerPubKey123", "1.2.3.4:51820", "10.42.0.2/32")
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	if wg.PeerCount() != 1 {
		t.Errorf("expected 1 peer, got %d", wg.PeerCount())
	}

	// Adding same peer again should be idempotent
	wg.AddPeer("peerPubKey123", "1.2.3.4:51820", "10.42.0.2/32")
	if wg.PeerCount() != 1 {
		t.Errorf("expected 1 peer after duplicate add, got %d", wg.PeerCount())
	}

	// Remove peer
	wg.RemovePeer("peerPubKey123")
	if wg.PeerCount() != 0 {
		t.Errorf("expected 0 peers after remove, got %d", wg.PeerCount())
	}
}

func TestWireGuardManager_WriteConfig(t *testing.T) {
	dir := t.TempDir()
	wg := infra.NewWireGuardManager(dir)
	wg.Initialize(51820, "10.42.0.1/24")
	wg.AddPeer("peerPubKey123=", "1.2.3.4:51820", "10.42.0.2/32")

	configPath, err := wg.WriteConfig()
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Config file should exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", configPath)
	}

	// Should be in the config dir
	if filepath.Dir(configPath) != dir {
		t.Errorf("config not in expected dir: %s", configPath)
	}

	// Read and verify contents
	content, _ := os.ReadFile(configPath)
	configStr := string(content)

	if !strings.Contains(configStr, "[Interface]") {
		t.Error("config should contain [Interface] section")
	}
	if !strings.Contains(configStr, "[Peer]") {
		t.Error("config should contain [Peer] section")
	}
	if !strings.Contains(configStr, "ListenPort = 51820") {
		t.Error("config should contain listen port")
	}
	if !strings.Contains(configStr, "peerPubKey123=") {
		t.Error("config should contain peer public key")
	}
}

func TestAllocateMeshIP(t *testing.T) {
	tests := []struct {
		index    int
		expected string
	}{
		{0, "10.42.0.1/24"},
		{1, "10.42.0.2/24"},
		{5, "10.42.0.6/24"},
	}

	for _, tt := range tests {
		ip := infra.AllocateMeshIP(tt.index)
		if ip != tt.expected {
			t.Errorf("AllocateMeshIP(%d) = %q, want %q", tt.index, ip, tt.expected)
		}
	}
}
