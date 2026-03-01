package infra

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/logger"
)

// NATType represents the detected NAT type.
type NATType string

const (
	NATTypeNone       NATType = "none"
	NATTypeFullCone   NATType = "full_cone"
	NATTypeRestricted NATType = "restricted"
	NATTypeSymmetric  NATType = "symmetric"
)

// STUN protocol constants (RFC 5389).
const (
	stunHeaderSize     = 20
	stunMagicCookie    = 0x2112A442
	stunBindingRequest = 0x0001
	stunBindingSuccess = 0x0101
	stunTimeout        = 3 * time.Second

	// Attribute types.
	stunAttrMappedAddress    = 0x0001
	stunAttrXORMappedAddress = 0x0020
)

// TURN protocol constants (RFC 5766).
const (
	turnAllocateRequest     = 0x0003
	turnAllocateSuccess     = 0x0103
	turnAllocateError       = 0x0113
	turnRefreshRequest      = 0x0004
	turnRefreshSuccess      = 0x0104
	turnPermissionRequest   = 0x0008
	turnPermissionSuccess   = 0x0108
	turnSendIndication      = 0x0016
	turnDataIndication      = 0x0017

	// TURN attribute types.
	turnAttrLifetime         = 0x000D
	turnAttrXORRelayedAddr   = 0x0016
	turnAttrRequestedTransport = 0x0019
	turnAttrXORPeerAddr      = 0x0012
	turnAttrData             = 0x0013
	turnAttrNonce            = 0x0015
	turnAttrRealm            = 0x0014
	turnAttrUsername          = 0x0006
	turnAttrMessageIntegrity = 0x0008
	turnAttrErrorCode        = 0x0009

	// TURN transport IDs.
	turnTransportUDP = 17

	// TURN default allocation lifetime.
	turnDefaultLifetime = 600 // 10 minutes
	turnRefreshInterval = 240 * time.Second // Refresh before half-lifetime
)

// TURNConfig holds TURN server connection parameters.
type TURNConfig struct {
	Server string // e.g., "turn.example.com:3478"
	User   string
	Pass   string
}

// TURNAllocation represents an active TURN relay allocation.
type TURNAllocation struct {
	RelayedAddr string        // Allocated relay address (IP:port)
	MappedAddr  string        // Server-reflexive address
	Lifetime    time.Duration // Allocation lifetime
	ExpiresAt   time.Time
	Nonce       string
	Realm       string
}

// TURNClient manages a TURN relay allocation for symmetric NAT traversal.
type TURNClient struct {
	serverAddr string
	username   string
	password   string
	conn       net.PacketConn
	allocation *TURNAllocation
	stopCh     chan struct{}
}

// NATTraversal provides STUN-based external endpoint discovery, NAT type detection,
// and TURN relay fallback for symmetric NAT.
type NATTraversal struct {
	stunServers []string
	turnClient  *TURNClient // nil if TURN not configured
}

// NewNATTraversal creates a new NATTraversal with the given STUN servers and optional TURN config.
func NewNATTraversal(stunServers []string, turnCfg ...TURNConfig) *NATTraversal {
	if len(stunServers) == 0 {
		stunServers = []string{"stun.l.google.com:19302"}
	}
	nat := &NATTraversal{
		stunServers: stunServers,
	}

	if len(turnCfg) > 0 && turnCfg[0].Server != "" {
		nat.turnClient = NewTURNClient(turnCfg[0].Server, turnCfg[0].User, turnCfg[0].Pass)
		logger.Info("TURN relay configured",
			"component", "nat",
			"server", turnCfg[0].Server,
		)
	}

	return nat
}

// HasTURN returns whether a TURN relay server is configured.
func (n *NATTraversal) HasTURN() bool {
	return n.turnClient != nil
}

// AllocateRelay requests a TURN relay allocation. Returns the relay address.
func (n *NATTraversal) AllocateRelay(ctx context.Context) (string, error) {
	if n.turnClient == nil {
		return "", fmt.Errorf("TURN not configured")
	}
	return n.turnClient.Allocate(ctx)
}

// DeallocateRelay releases the TURN relay allocation.
func (n *NATTraversal) DeallocateRelay(ctx context.Context) error {
	if n.turnClient == nil {
		return nil
	}
	return n.turnClient.Deallocate(ctx)
}

// NewTURNClient creates a TURN client for the given server.
func NewTURNClient(serverAddr, username, password string) *TURNClient {
	return &TURNClient{
		serverAddr: serverAddr,
		username:   username,
		password:   password,
		stopCh:     make(chan struct{}),
	}
}

// DiscoverExternalEndpoint sends a STUN Binding Request to discover the public IP:port.
// localPort is the UDP port to bind locally (e.g., WireGuard port).
func (n *NATTraversal) DiscoverExternalEndpoint(ctx context.Context, localPort int) (string, NATType, error) {
	if len(n.stunServers) == 0 {
		return "", NATTypeNone, fmt.Errorf("no STUN servers configured")
	}

	// Try each STUN server until one succeeds
	var lastErr error
	for _, server := range n.stunServers {
		endpoint, err := n.stunRequest(ctx, server, localPort)
		if err != nil {
			lastErr = err
			logger.Debug("STUN request failed, trying next server",
				"component", "nat",
				"server", server,
				"error", err,
			)
			continue
		}

		// Detect NAT type
		natType, err := n.DetectNATType(ctx, localPort)
		if err != nil {
			logger.Warn("NAT type detection failed, defaulting to restricted",
				"component", "nat",
				"error", err,
			)
			natType = NATTypeRestricted
		}

		return endpoint, natType, nil
	}

	return "", NATTypeNone, fmt.Errorf("all STUN servers failed: %w", lastErr)
}

// DetectNATType compares responses from multiple STUN servers to classify the NAT.
// Requires at least 2 STUN servers for meaningful detection.
func (n *NATTraversal) DetectNATType(ctx context.Context, localPort int) (NATType, error) {
	if len(n.stunServers) < 2 {
		return NATTypeRestricted, nil // Cannot determine without multiple servers
	}

	// Query two different STUN servers from the same local port
	endpoint1, err := n.stunRequest(ctx, n.stunServers[0], localPort)
	if err != nil {
		return NATTypeNone, fmt.Errorf("STUN server 1: %w", err)
	}

	endpoint2, err := n.stunRequest(ctx, n.stunServers[1], localPort)
	if err != nil {
		return NATTypeNone, fmt.Errorf("STUN server 2: %w", err)
	}

	// Parse endpoints to compare
	host1, port1, err := net.SplitHostPort(endpoint1)
	if err != nil {
		return NATTypeNone, fmt.Errorf("parse endpoint1: %w", err)
	}
	host2, port2, err := net.SplitHostPort(endpoint2)
	if err != nil {
		return NATTypeNone, fmt.Errorf("parse endpoint2: %w", err)
	}

	// Same IP and port from different servers → full cone or no NAT
	if host1 == host2 && port1 == port2 {
		// Check if the external port matches local port (no NAT)
		if port1 == fmt.Sprintf("%d", localPort) {
			return NATTypeNone, nil
		}
		return NATTypeFullCone, nil
	}

	// Same IP but different port → restricted or port-restricted
	if host1 == host2 && port1 != port2 {
		return NATTypeRestricted, nil
	}

	// Different IP → symmetric NAT
	return NATTypeSymmetric, nil
}

// stunRequest sends a STUN Binding Request and returns the discovered endpoint.
func (n *NATTraversal) stunRequest(ctx context.Context, server string, localPort int) (string, error) {
	// Resolve STUN server address
	serverAddr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return "", fmt.Errorf("resolve STUN server %s: %w", server, err)
	}

	// Try binding to the specified local port first, fall back to ephemeral
	var localAddr *net.UDPAddr
	if localPort > 0 {
		localAddr = &net.UDPAddr{Port: localPort}
	}

	conn, err := net.DialUDP("udp4", localAddr, serverAddr)
	if err != nil && localPort > 0 {
		// Port might be in use (e.g., by WireGuard), try ephemeral port
		conn, err = net.DialUDP("udp4", nil, serverAddr)
	}
	if err != nil {
		return "", fmt.Errorf("dial STUN server: %w", err)
	}
	defer conn.Close()

	// Build STUN Binding Request (RFC 5389)
	request := buildSTUNBindingRequest()

	// Set deadline from context or default timeout
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(stunTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("set deadline: %w", err)
	}

	// Send request
	if _, err := conn.Write(request); err != nil {
		return "", fmt.Errorf("send STUN request: %w", err)
	}

	// Read response
	buf := make([]byte, 1024)
	readN, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read STUN response: %w", err)
	}

	// Parse response
	ip, port, err := parseSTUNResponse(buf[:readN], request[8:20])
	if err != nil {
		return "", fmt.Errorf("parse STUN response: %w", err)
	}

	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

// buildSTUNBindingRequest creates a STUN Binding Request per RFC 5389.
// Header layout (20 bytes):
//   - Type:           2 bytes (0x0001 = Binding Request)
//   - Length:          2 bytes (0x0000 = no attributes)
//   - Magic Cookie:   4 bytes (0x2112A442)
//   - Transaction ID: 12 bytes (random)
func buildSTUNBindingRequest() []byte {
	msg := make([]byte, stunHeaderSize)

	// Message type: Binding Request
	binary.BigEndian.PutUint16(msg[0:2], stunBindingRequest)

	// Message length: 0 (no attributes in request)
	binary.BigEndian.PutUint16(msg[2:4], 0)

	// Magic cookie
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)

	// Transaction ID (12 random bytes)
	if _, err := rand.Read(msg[8:20]); err != nil {
		// Fallback: use timestamp-based ID (non-critical)
		binary.BigEndian.PutUint64(msg[8:16], uint64(time.Now().UnixNano()))
	}

	return msg
}

// parseSTUNResponse parses a STUN Binding Success Response and extracts the mapped address.
// It looks for XOR-MAPPED-ADDRESS (0x0020) first, then falls back to MAPPED-ADDRESS (0x0001).
func parseSTUNResponse(data []byte, transactionID []byte) (net.IP, int, error) {
	if len(data) < stunHeaderSize {
		return nil, 0, fmt.Errorf("response too short: %d bytes", len(data))
	}

	// Verify message type is Binding Success Response
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != stunBindingSuccess {
		return nil, 0, fmt.Errorf("unexpected STUN message type: 0x%04x", msgType)
	}

	// Verify magic cookie
	cookie := binary.BigEndian.Uint32(data[4:8])
	if cookie != stunMagicCookie {
		return nil, 0, fmt.Errorf("invalid magic cookie: 0x%08x", cookie)
	}

	// Parse attributes
	attrStart := stunHeaderSize
	msgLen := int(binary.BigEndian.Uint16(data[2:4]))
	if attrStart+msgLen > len(data) {
		return nil, 0, fmt.Errorf("message length exceeds data: %d > %d", attrStart+msgLen, len(data))
	}

	var mappedIP net.IP
	var mappedPort int
	foundXOR := false

	pos := attrStart
	end := attrStart + msgLen
	for pos+4 <= end {
		attrType := binary.BigEndian.Uint16(data[pos : pos+2])
		attrLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+attrLen > end {
			break
		}

		switch attrType {
		case stunAttrXORMappedAddress:
			ip, port, err := parseXORMappedAddress(data[pos:pos+attrLen], transactionID)
			if err == nil {
				mappedIP = ip
				mappedPort = port
				foundXOR = true
			}
		case stunAttrMappedAddress:
			if !foundXOR {
				ip, port, err := parseMappedAddress(data[pos : pos+attrLen])
				if err == nil {
					mappedIP = ip
					mappedPort = port
				}
			}
		}

		// Attributes are padded to 4-byte boundaries
		pos += attrLen
		if pad := attrLen % 4; pad != 0 {
			pos += 4 - pad
		}
	}

	if mappedIP == nil {
		return nil, 0, fmt.Errorf("no mapped address found in STUN response")
	}

	return mappedIP, mappedPort, nil
}

// parseXORMappedAddress decodes an XOR-MAPPED-ADDRESS attribute (RFC 5389 Section 15.2).
// Port is XOR'd with the high 16 bits of the magic cookie.
// IPv4 address is XOR'd with the full magic cookie.
func parseXORMappedAddress(data []byte, transactionID []byte) (net.IP, int, error) {
	if len(data) < 8 {
		return nil, 0, fmt.Errorf("XOR-MAPPED-ADDRESS too short: %d bytes", len(data))
	}

	family := data[1]
	if family != 0x01 { // IPv4
		return nil, 0, fmt.Errorf("unsupported address family: 0x%02x (only IPv4 supported)", family)
	}

	// XOR port with magic cookie high 16 bits (0x2112)
	xPort := binary.BigEndian.Uint16(data[2:4])
	port := xPort ^ uint16(stunMagicCookie>>16)

	// XOR IPv4 with magic cookie
	var ip [4]byte
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		ip[i] = data[4+i] ^ magicBytes[i]
	}

	return net.IPv4(ip[0], ip[1], ip[2], ip[3]), int(port), nil
}

// parseMappedAddress decodes a MAPPED-ADDRESS attribute (RFC 5389 Section 15.1).
func parseMappedAddress(data []byte) (net.IP, int, error) {
	if len(data) < 8 {
		return nil, 0, fmt.Errorf("MAPPED-ADDRESS too short: %d bytes", len(data))
	}

	family := data[1]
	if family != 0x01 { // IPv4
		return nil, 0, fmt.Errorf("unsupported address family: 0x%02x", family)
	}

	port := int(binary.BigEndian.Uint16(data[2:4]))
	ip := net.IPv4(data[4], data[5], data[6], data[7])

	return ip, port, nil
}

// ===== TURN Client (RFC 5766) =====

// Allocate requests a relay allocation from the TURN server.
// Returns the relay address (IP:port) for WireGuard handshake.
func (tc *TURNClient) Allocate(ctx context.Context) (string, error) {
	serverAddr, err := net.ResolveUDPAddr("udp4", tc.serverAddr)
	if err != nil {
		return "", fmt.Errorf("resolve TURN server %s: %w", tc.serverAddr, err)
	}

	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return "", fmt.Errorf("listen for TURN: %w", err)
	}
	tc.conn = conn

	// Step 1: Send unauthenticated Allocate — expect 401 with nonce+realm
	txnID := generateTxnID()
	allocReq := buildTURNAllocateRequest(txnID)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return "", fmt.Errorf("set deadline: %w", err)
	}

	if _, err := conn.WriteTo(allocReq, serverAddr); err != nil {
		conn.Close()
		return "", fmt.Errorf("send allocate request: %w", err)
	}

	buf := make([]byte, 2048)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("read allocate response: %w", err)
	}

	// Parse 401 error response to extract nonce and realm
	nonce, realm, err := parseTURN401Response(buf[:n])
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("parse TURN 401: %w", err)
	}

	// Step 2: Send authenticated Allocate with credentials
	txnID = generateTxnID()
	authAllocReq := buildTURNAuthAllocateRequest(txnID, tc.username, realm, nonce, tc.password)

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return "", fmt.Errorf("set deadline: %w", err)
	}

	if _, err := conn.WriteTo(authAllocReq, serverAddr); err != nil {
		conn.Close()
		return "", fmt.Errorf("send auth allocate: %w", err)
	}

	n, _, err = conn.ReadFrom(buf)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("read auth allocate response: %w", err)
	}

	// Parse success response
	relayAddr, mappedAddr, lifetime, err := parseTURNAllocateSuccess(buf[:n], txnID)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("parse allocate success: %w", err)
	}

	tc.allocation = &TURNAllocation{
		RelayedAddr: relayAddr,
		MappedAddr:  mappedAddr,
		Lifetime:    time.Duration(lifetime) * time.Second,
		ExpiresAt:   time.Now().Add(time.Duration(lifetime) * time.Second),
		Nonce:       nonce,
		Realm:       realm,
	}

	logger.Info("TURN relay allocated",
		"component", "nat",
		"relay_addr", relayAddr,
		"mapped_addr", mappedAddr,
		"lifetime_s", lifetime,
	)

	// Start refresh goroutine to keep allocation alive
	go tc.refreshLoop(serverAddr)

	return relayAddr, nil
}

// CreatePermission allows traffic from a specific peer IP through the relay.
func (tc *TURNClient) CreatePermission(ctx context.Context, peerIP string) error {
	if tc.conn == nil || tc.allocation == nil {
		return fmt.Errorf("no active TURN allocation")
	}

	ip := net.ParseIP(peerIP)
	if ip == nil {
		return fmt.Errorf("invalid peer IP: %s", peerIP)
	}

	serverAddr, err := net.ResolveUDPAddr("udp4", tc.serverAddr)
	if err != nil {
		return fmt.Errorf("resolve TURN server: %w", err)
	}

	txnID := generateTxnID()
	permReq := buildTURNPermissionRequest(txnID, ip, tc.username, tc.allocation.Realm, tc.allocation.Nonce, tc.password)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := tc.conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	if _, err := tc.conn.WriteTo(permReq, serverAddr); err != nil {
		return fmt.Errorf("send permission request: %w", err)
	}

	buf := make([]byte, 1024)
	n, _, err := tc.conn.ReadFrom(buf)
	if err != nil {
		return fmt.Errorf("read permission response: %w", err)
	}

	msgType := binary.BigEndian.Uint16(buf[:2])
	if msgType != turnPermissionSuccess {
		return fmt.Errorf("TURN permission request failed, response type: 0x%04x", msgType)
	}

	_ = n
	logger.Info("TURN permission created",
		"component", "nat",
		"peer_ip", peerIP,
	)

	return nil
}

// Deallocate releases the TURN relay allocation.
func (tc *TURNClient) Deallocate(_ context.Context) error {
	close(tc.stopCh)

	if tc.conn != nil {
		tc.conn.Close()
		tc.conn = nil
	}

	tc.allocation = nil
	logger.Info("TURN relay deallocated", "component", "nat")
	return nil
}

// refreshLoop periodically refreshes the TURN allocation before it expires.
func (tc *TURNClient) refreshLoop(serverAddr *net.UDPAddr) {
	ticker := time.NewTicker(turnRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tc.stopCh:
			return
		case <-ticker.C:
			if tc.conn == nil || tc.allocation == nil {
				return
			}

			txnID := generateTxnID()
			refreshReq := buildTURNRefreshRequest(txnID, turnDefaultLifetime, tc.username, tc.allocation.Realm, tc.allocation.Nonce, tc.password)

			if err := tc.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				logger.Warn("TURN refresh deadline failed", "error", err)
				continue
			}

			if _, err := tc.conn.WriteTo(refreshReq, serverAddr); err != nil {
				logger.Warn("TURN refresh send failed", "error", err)
				continue
			}

			buf := make([]byte, 1024)
			_, _, err := tc.conn.ReadFrom(buf)
			if err != nil {
				logger.Warn("TURN refresh response failed", "error", err)
				continue
			}

			tc.allocation.ExpiresAt = time.Now().Add(time.Duration(turnDefaultLifetime) * time.Second)
			logger.Debug("TURN allocation refreshed", "component", "nat")
		}
	}
}

// ===== TURN message builders =====

var turnTxnMu sync.Mutex

func generateTxnID() []byte {
	txnID := make([]byte, 12)
	turnTxnMu.Lock()
	defer turnTxnMu.Unlock()
	if _, err := rand.Read(txnID); err != nil {
		binary.BigEndian.PutUint64(txnID, uint64(time.Now().UnixNano()))
	}
	return txnID
}

func buildTURNAllocateRequest(txnID []byte) []byte {
	// STUN/TURN header (20 bytes) + REQUESTED-TRANSPORT attribute (8 bytes)
	msg := make([]byte, 28)

	binary.BigEndian.PutUint16(msg[0:2], turnAllocateRequest)
	binary.BigEndian.PutUint16(msg[2:4], 8) // Length of attributes
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txnID)

	// REQUESTED-TRANSPORT: UDP (17)
	binary.BigEndian.PutUint16(msg[20:22], turnAttrRequestedTransport)
	binary.BigEndian.PutUint16(msg[22:24], 4)
	msg[24] = turnTransportUDP // Protocol number
	// msg[25:28] are reserved (zero)

	return msg
}

func buildTURNAuthAllocateRequest(txnID []byte, username, realm, nonce, password string) []byte {
	// Build attributes
	var attrs []byte

	// REQUESTED-TRANSPORT
	attrs = append(attrs, encodeAttr(turnAttrRequestedTransport, []byte{turnTransportUDP, 0, 0, 0})...)

	// USERNAME
	attrs = append(attrs, encodeAttr(turnAttrUsername, []byte(username))...)

	// REALM
	attrs = append(attrs, encodeAttr(turnAttrRealm, []byte(realm))...)

	// NONCE
	attrs = append(attrs, encodeAttr(turnAttrNonce, []byte(nonce))...)

	// LIFETIME
	lifetimeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lifetimeBuf, turnDefaultLifetime)
	attrs = append(attrs, encodeAttr(turnAttrLifetime, lifetimeBuf)...)

	// MESSAGE-INTEGRITY (HMAC-SHA1)
	key := turnLongTermKey(username, realm, password)
	msg := buildSTUNMessage(turnAllocateRequest, txnID, attrs)
	integrity := computeMessageIntegrity(msg, key)
	attrs = append(attrs, encodeAttr(turnAttrMessageIntegrity, integrity)...)

	return buildSTUNMessage(turnAllocateRequest, txnID, attrs)
}

func buildTURNPermissionRequest(txnID []byte, peerIP net.IP, username, realm, nonce, password string) []byte {
	var attrs []byte

	// XOR-PEER-ADDRESS
	xorAddr := encodeXORAddress(peerIP, 0, txnID)
	attrs = append(attrs, encodeAttr(turnAttrXORPeerAddr, xorAddr)...)

	// USERNAME
	attrs = append(attrs, encodeAttr(turnAttrUsername, []byte(username))...)

	// REALM
	attrs = append(attrs, encodeAttr(turnAttrRealm, []byte(realm))...)

	// NONCE
	attrs = append(attrs, encodeAttr(turnAttrNonce, []byte(nonce))...)

	// MESSAGE-INTEGRITY
	key := turnLongTermKey(username, realm, password)
	msg := buildSTUNMessage(turnPermissionRequest, txnID, attrs)
	integrity := computeMessageIntegrity(msg, key)
	attrs = append(attrs, encodeAttr(turnAttrMessageIntegrity, integrity)...)

	return buildSTUNMessage(turnPermissionRequest, txnID, attrs)
}

func buildTURNRefreshRequest(txnID []byte, lifetime uint32, username, realm, nonce, password string) []byte {
	var attrs []byte

	// LIFETIME
	lifetimeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lifetimeBuf, lifetime)
	attrs = append(attrs, encodeAttr(turnAttrLifetime, lifetimeBuf)...)

	// USERNAME
	attrs = append(attrs, encodeAttr(turnAttrUsername, []byte(username))...)

	// REALM
	attrs = append(attrs, encodeAttr(turnAttrRealm, []byte(realm))...)

	// NONCE
	attrs = append(attrs, encodeAttr(turnAttrNonce, []byte(nonce))...)

	// MESSAGE-INTEGRITY
	key := turnLongTermKey(username, realm, password)
	msg := buildSTUNMessage(turnRefreshRequest, txnID, attrs)
	integrity := computeMessageIntegrity(msg, key)
	attrs = append(attrs, encodeAttr(turnAttrMessageIntegrity, integrity)...)

	return buildSTUNMessage(turnRefreshRequest, txnID, attrs)
}

// ===== TURN response parsers =====

func parseTURN401Response(data []byte) (nonce, realm string, err error) {
	if len(data) < stunHeaderSize {
		return "", "", fmt.Errorf("response too short: %d", len(data))
	}

	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != turnAllocateError {
		return "", "", fmt.Errorf("expected 401 error (0x%04x), got 0x%04x", turnAllocateError, msgType)
	}

	msgLen := int(binary.BigEndian.Uint16(data[2:4]))
	pos := stunHeaderSize
	end := stunHeaderSize + msgLen
	if end > len(data) {
		end = len(data)
	}

	for pos+4 <= end {
		attrType := binary.BigEndian.Uint16(data[pos : pos+2])
		attrLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+attrLen > end {
			break
		}

		switch attrType {
		case turnAttrNonce:
			nonce = string(data[pos : pos+attrLen])
		case turnAttrRealm:
			realm = string(data[pos : pos+attrLen])
		}

		pos += attrLen
		if pad := attrLen % 4; pad != 0 {
			pos += 4 - pad
		}
	}

	if nonce == "" || realm == "" {
		return "", "", fmt.Errorf("missing nonce or realm in 401 response")
	}

	return nonce, realm, nil
}

func parseTURNAllocateSuccess(data []byte, txnID []byte) (relayAddr, mappedAddr string, lifetime int, err error) {
	if len(data) < stunHeaderSize {
		return "", "", 0, fmt.Errorf("response too short: %d", len(data))
	}

	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != turnAllocateSuccess {
		return "", "", 0, fmt.Errorf("expected allocate success (0x%04x), got 0x%04x", turnAllocateSuccess, msgType)
	}

	msgLen := int(binary.BigEndian.Uint16(data[2:4]))
	pos := stunHeaderSize
	end := stunHeaderSize + msgLen
	if end > len(data) {
		end = len(data)
	}

	for pos+4 <= end {
		attrType := binary.BigEndian.Uint16(data[pos : pos+2])
		attrLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+attrLen > end {
			break
		}

		switch attrType {
		case turnAttrXORRelayedAddr:
			ip, port, parseErr := parseXORMappedAddress(data[pos:pos+attrLen], txnID)
			if parseErr == nil {
				relayAddr = fmt.Sprintf("%s:%d", ip, port)
			}
		case stunAttrXORMappedAddress:
			ip, port, parseErr := parseXORMappedAddress(data[pos:pos+attrLen], txnID)
			if parseErr == nil {
				mappedAddr = fmt.Sprintf("%s:%d", ip, port)
			}
		case turnAttrLifetime:
			if attrLen >= 4 {
				lifetime = int(binary.BigEndian.Uint32(data[pos : pos+4]))
			}
		}

		pos += attrLen
		if pad := attrLen % 4; pad != 0 {
			pos += 4 - pad
		}
	}

	if relayAddr == "" {
		return "", "", 0, fmt.Errorf("no relay address in allocate success")
	}
	if lifetime == 0 {
		lifetime = turnDefaultLifetime
	}

	return relayAddr, mappedAddr, lifetime, nil
}

// ===== TURN helpers =====

func buildSTUNMessage(msgType uint16, txnID []byte, attrs []byte) []byte {
	msg := make([]byte, stunHeaderSize+len(attrs))
	binary.BigEndian.PutUint16(msg[0:2], msgType)
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(attrs)))
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txnID)
	copy(msg[20:], attrs)
	return msg
}

func encodeAttr(attrType uint16, value []byte) []byte {
	// 4 bytes header + value + padding to 4-byte boundary
	padded := len(value)
	if rem := padded % 4; rem != 0 {
		padded += 4 - rem
	}
	attr := make([]byte, 4+padded)
	binary.BigEndian.PutUint16(attr[0:2], attrType)
	binary.BigEndian.PutUint16(attr[2:4], uint16(len(value)))
	copy(attr[4:], value)
	return attr
}

func encodeXORAddress(ip net.IP, port int, txnID []byte) []byte {
	buf := make([]byte, 8)
	buf[0] = 0    // Reserved
	buf[1] = 0x01 // IPv4

	// XOR port with magic cookie high 16 bits
	xPort := uint16(port) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(buf[2:4], xPort)

	// XOR IPv4 with magic cookie
	ip4 := ip.To4()
	if ip4 == nil {
		return buf
	}
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		buf[4+i] = ip4[i] ^ magicBytes[i]
	}

	return buf
}

// turnLongTermKey computes the TURN long-term credential key (RFC 5389 §10.2).
// key = MD5(username:realm:password)
func turnLongTermKey(username, realm, password string) []byte {
	h := md5.Sum([]byte(username + ":" + realm + ":" + password))
	return h[:]
}

// computeMessageIntegrity computes HMAC-SHA1 over the STUN message.
// The message length in the header must be adjusted to include the integrity attribute.
func computeMessageIntegrity(msg []byte, key []byte) []byte {
	// Adjust message length to include the integrity attribute (24 bytes: 4 header + 20 HMAC)
	adjustedLen := len(msg) - stunHeaderSize + 24
	adjusted := make([]byte, len(msg))
	copy(adjusted, msg)
	binary.BigEndian.PutUint16(adjusted[2:4], uint16(adjustedLen))

	mac := hmac.New(sha1.New, key)
	mac.Write(adjusted)
	return mac.Sum(nil)
}
