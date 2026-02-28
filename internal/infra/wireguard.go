package infra

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"github.com/joaopedro/hivemind/internal/logger"
	"golang.org/x/crypto/curve25519"
)

// WireGuardConfig holds the configuration for a WireGuard interface.
type WireGuardConfig struct {
	InterfaceName string
	ListenPort    int
	PrivateKey    string
	PublicKey     string
	Address       string // e.g., "10.42.0.1/24"
	Peers         []WireGuardPeer
}

// WireGuardPeer represents a WireGuard peer.
type WireGuardPeer struct {
	PublicKey  string
	Endpoint  string // e.g., "1.2.3.4:51820"
	AllowedIP string // e.g., "10.42.0.2/32"
}

// WireGuardManager manages the WireGuard mesh for a room.
type WireGuardManager struct {
	mu        sync.RWMutex
	config    *WireGuardConfig
	configDir string
	isUp      bool
}

// NewWireGuardManager creates a new WireGuard manager.
func NewWireGuardManager(configDir string) *WireGuardManager {
	return &WireGuardManager{
		configDir: configDir,
	}
}

// GenerateKeyPair generates a WireGuard Curve25519 keypair.
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	var private [32]byte
	if _, err := rand.Read(private[:]); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp private key per Curve25519 spec
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	var public [32]byte
	curve25519.ScalarBaseMult(&public, &private)

	return base64.StdEncoding.EncodeToString(private[:]),
		base64.StdEncoding.EncodeToString(public[:]),
		nil
}

// Initialize generates keys and prepares the WireGuard config.
func (wg *WireGuardManager) Initialize(listenPort int, meshIP string) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	privKey, pubKey, err := GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate keypair: %w", err)
	}

	wg.config = &WireGuardConfig{
		InterfaceName: "hm0",
		ListenPort:    listenPort,
		PrivateKey:    privKey,
		PublicKey:      pubKey,
		Address:       meshIP,
		Peers:         []WireGuardPeer{},
	}

	logger.Info("wireguard initialized",
		"interface", wg.config.InterfaceName,
		"port", listenPort,
		"public_key", pubKey[:8]+"...",
		"mesh_ip", meshIP,
	)

	return nil
}

// AddPeer adds a peer to the WireGuard mesh.
func (wg *WireGuardManager) AddPeer(pubKey, endpoint, allowedIP string) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if wg.config == nil {
		return fmt.Errorf("wireguard not initialized")
	}

	// Check for duplicate
	for _, p := range wg.config.Peers {
		if p.PublicKey == pubKey {
			return nil // Already added
		}
	}

	wg.config.Peers = append(wg.config.Peers, WireGuardPeer{
		PublicKey:  pubKey,
		Endpoint:  endpoint,
		AllowedIP: allowedIP,
	})

	logger.Info("peer added to wireguard mesh",
		"public_key", pubKey[:8]+"...",
		"endpoint", endpoint,
		"allowed_ip", allowedIP,
	)

	return nil
}

// RemovePeer removes a peer from the WireGuard mesh.
func (wg *WireGuardManager) RemovePeer(pubKey string) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if wg.config == nil {
		return fmt.Errorf("wireguard not initialized")
	}

	for i, p := range wg.config.Peers {
		if p.PublicKey == pubKey {
			wg.config.Peers = append(wg.config.Peers[:i], wg.config.Peers[i+1:]...)
			logger.Info("peer removed from mesh", "public_key", pubKey[:8]+"...")
			return nil
		}
	}

	return nil
}

// PublicKey returns this node's WireGuard public key.
func (wg *WireGuardManager) PublicKey() string {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	if wg.config == nil {
		return ""
	}
	return wg.config.PublicKey
}

// MeshAddress returns this node's mesh IP address.
func (wg *WireGuardManager) MeshAddress() string {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	if wg.config == nil {
		return ""
	}
	return wg.config.Address
}

// WriteConfig writes the WireGuard config file.
func (wg *WireGuardManager) WriteConfig() (string, error) {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	if wg.config == nil {
		return "", fmt.Errorf("wireguard not initialized")
	}

	if err := os.MkdirAll(wg.configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config dir: %w", err)
	}

	configPath := filepath.Join(wg.configDir, wg.config.InterfaceName+".conf")

	f, err := os.OpenFile(configPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	if err := wgConfigTemplate.Execute(f, wg.config); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	logger.Info("wireguard config written", "path", configPath)
	return configPath, nil
}

// PeerCount returns the number of configured peers.
func (wg *WireGuardManager) PeerCount() int {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	if wg.config == nil {
		return 0
	}
	return len(wg.config.Peers)
}

// AllocateMeshIP allocates a mesh IP for a peer based on their index.
func AllocateMeshIP(peerIndex int) string {
	return fmt.Sprintf("10.42.0.%d/24", peerIndex+1)
}

// GetLocalIP returns the best non-loopback IP address.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}

var wgConfigTemplate = template.Must(template.New("wg").Parse(`[Interface]
PrivateKey = {{ .PrivateKey }}
Address = {{ .Address }}
ListenPort = {{ .ListenPort }}
{{ range .Peers }}
[Peer]
PublicKey = {{ .PublicKey }}
Endpoint = {{ .Endpoint }}
AllowedIPs = {{ .AllowedIP }}
PersistentKeepalive = 25
{{ end }}`))
