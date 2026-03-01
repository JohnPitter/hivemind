package infra

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// startMockSTUNServer starts a UDP server that responds to STUN Binding Requests
// with a configurable mapped address. Returns the server address and a cleanup function.
func startMockSTUNServer(t *testing.T, mappedIP net.IP, mappedPort int) (string, func()) {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to start mock STUN server: %v", err)
	}

	// Use 127.0.0.1 explicitly for the returned address
	_, portStr, _ := net.SplitHostPort(conn.LocalAddr().String())
	addr := "127.0.0.1:" + portStr

	go func() {
		buf := make([]byte, 1024)
		for {
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // server closed
			}
			if n < stunHeaderSize {
				continue
			}

			// Verify it's a Binding Request
			msgType := binary.BigEndian.Uint16(buf[0:2])
			if msgType != stunBindingRequest {
				continue
			}

			// Extract transaction ID from request
			transactionID := make([]byte, 12)
			copy(transactionID, buf[8:20])

			// Build response with XOR-MAPPED-ADDRESS
			resp := buildMockSTUNResponse(transactionID, mappedIP, mappedPort)
			conn.WriteToUDP(resp, remoteAddr)
		}
	}()

	return addr, func() { conn.Close() }
}

// buildMockSTUNResponse builds a STUN Binding Success Response with XOR-MAPPED-ADDRESS.
func buildMockSTUNResponse(transactionID []byte, ip net.IP, port int) []byte {
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4(127, 0, 0, 1).To4()
	}

	// XOR-MAPPED-ADDRESS attribute (12 bytes: 4 header + 8 value)
	attr := make([]byte, 12)
	// Attribute type: XOR-MAPPED-ADDRESS
	binary.BigEndian.PutUint16(attr[0:2], stunAttrXORMappedAddress)
	// Attribute length: 8 bytes
	binary.BigEndian.PutUint16(attr[2:4], 8)
	// Reserved byte
	attr[4] = 0x00
	// Family: IPv4
	attr[5] = 0x01
	// XOR'd port (port XOR magic cookie high 16 bits)
	xPort := uint16(port) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(attr[6:8], xPort)
	// XOR'd IP (IP XOR magic cookie)
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	for i := 0; i < 4; i++ {
		attr[8+i] = ip4[i] ^ magicBytes[i]
	}

	// Build full response
	resp := make([]byte, stunHeaderSize+len(attr))
	// Message type: Binding Success Response
	binary.BigEndian.PutUint16(resp[0:2], stunBindingSuccess)
	// Message length
	binary.BigEndian.PutUint16(resp[2:4], uint16(len(attr)))
	// Magic cookie
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	// Transaction ID
	copy(resp[8:20], transactionID)
	// Attribute
	copy(resp[20:], attr)

	return resp
}

func TestBuildSTUNBindingRequest(t *testing.T) {
	req := buildSTUNBindingRequest()

	if len(req) != stunHeaderSize {
		t.Fatalf("expected %d bytes, got %d", stunHeaderSize, len(req))
	}

	// Check message type
	msgType := binary.BigEndian.Uint16(req[0:2])
	if msgType != stunBindingRequest {
		t.Errorf("expected message type 0x%04x, got 0x%04x", stunBindingRequest, msgType)
	}

	// Check message length
	msgLen := binary.BigEndian.Uint16(req[2:4])
	if msgLen != 0 {
		t.Errorf("expected message length 0, got %d", msgLen)
	}

	// Check magic cookie
	cookie := binary.BigEndian.Uint32(req[4:8])
	if cookie != stunMagicCookie {
		t.Errorf("expected magic cookie 0x%08x, got 0x%08x", stunMagicCookie, cookie)
	}

	// Transaction ID should not be all zeros
	allZero := true
	for _, b := range req[8:20] {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("transaction ID should not be all zeros")
	}
}

func TestParseXORMappedAddress(t *testing.T) {
	expectedIP := net.IPv4(203, 0, 113, 42)
	expectedPort := 12345

	// Build XOR-MAPPED-ADDRESS value (8 bytes)
	data := make([]byte, 8)
	data[0] = 0x00 // reserved
	data[1] = 0x01 // IPv4

	// XOR port
	xPort := uint16(expectedPort) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(data[2:4], xPort)

	// XOR IP
	magicBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(magicBytes, stunMagicCookie)
	ip4 := expectedIP.To4()
	for i := 0; i < 4; i++ {
		data[4+i] = ip4[i] ^ magicBytes[i]
	}

	ip, port, err := parseXORMappedAddress(data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ip.Equal(expectedIP) {
		t.Errorf("expected IP %s, got %s", expectedIP, ip)
	}
	if port != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, port)
	}
}

func TestParseMappedAddress(t *testing.T) {
	expectedIP := net.IPv4(192, 168, 1, 100)
	expectedPort := 54321

	data := make([]byte, 8)
	data[0] = 0x00 // reserved
	data[1] = 0x01 // IPv4
	binary.BigEndian.PutUint16(data[2:4], uint16(expectedPort))
	ip4 := expectedIP.To4()
	copy(data[4:8], ip4)

	ip, port, err := parseMappedAddress(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ip.Equal(expectedIP) {
		t.Errorf("expected IP %s, got %s", expectedIP, ip)
	}
	if port != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, port)
	}
}

func TestParseSTUNResponse(t *testing.T) {
	expectedIP := net.IPv4(198, 51, 100, 5)
	expectedPort := 9999
	transactionID := make([]byte, 12)
	for i := range transactionID {
		transactionID[i] = byte(i + 1)
	}

	resp := buildMockSTUNResponse(transactionID, expectedIP, expectedPort)

	ip, port, err := parseSTUNResponse(resp, transactionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ip.Equal(expectedIP) {
		t.Errorf("expected IP %s, got %s", expectedIP, ip)
	}
	if port != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, port)
	}
}

func TestParseSTUNResponse_InvalidType(t *testing.T) {
	data := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(data[0:2], 0x0111) // wrong type
	binary.BigEndian.PutUint32(data[4:8], stunMagicCookie)

	_, _, err := parseSTUNResponse(data, nil)
	if err == nil {
		t.Error("expected error for wrong message type")
	}
}

func TestParseSTUNResponse_TooShort(t *testing.T) {
	data := make([]byte, 10) // too short
	_, _, err := parseSTUNResponse(data, nil)
	if err == nil {
		t.Error("expected error for short response")
	}
}

func TestNATTraversal_MockServer(t *testing.T) {
	expectedIP := net.IPv4(203, 0, 113, 1)
	expectedPort := 51820

	addr, cleanup := startMockSTUNServer(t, expectedIP, expectedPort)
	defer cleanup()

	nat := NewNATTraversal([]string{addr})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use port 0 to let the OS assign an ephemeral port
	endpoint, _, err := nat.DiscoverExternalEndpoint(ctx, 0)
	if err != nil {
		t.Fatalf("DiscoverExternalEndpoint failed: %v", err)
	}

	expected := "203.0.113.1:51820"
	if endpoint != expected {
		t.Errorf("expected endpoint %s, got %s", expected, endpoint)
	}
}

func TestNATTraversal_DetectNATType_FullCone(t *testing.T) {
	// Both servers return same IP:port → full cone
	ip := net.IPv4(203, 0, 113, 1)
	port := 40000

	addr1, cleanup1 := startMockSTUNServer(t, ip, port)
	defer cleanup1()
	addr2, cleanup2 := startMockSTUNServer(t, ip, port)
	defer cleanup2()

	nat := NewNATTraversal([]string{addr1, addr2})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	natType, err := nat.DetectNATType(ctx, 0)
	if err != nil {
		t.Fatalf("DetectNATType failed: %v", err)
	}

	if natType != NATTypeFullCone {
		t.Errorf("expected NAT type %s, got %s", NATTypeFullCone, natType)
	}
}

func TestNATTraversal_DetectNATType_Symmetric(t *testing.T) {
	// Servers return different IPs → symmetric NAT
	addr1, cleanup1 := startMockSTUNServer(t, net.IPv4(203, 0, 113, 1), 40000)
	defer cleanup1()
	addr2, cleanup2 := startMockSTUNServer(t, net.IPv4(198, 51, 100, 1), 40000)
	defer cleanup2()

	nat := NewNATTraversal([]string{addr1, addr2})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	natType, err := nat.DetectNATType(ctx, 0)
	if err != nil {
		t.Fatalf("DetectNATType failed: %v", err)
	}

	if natType != NATTypeSymmetric {
		t.Errorf("expected NAT type %s, got %s", NATTypeSymmetric, natType)
	}
}

func TestNATTraversal_NoServers(t *testing.T) {
	nat := &NATTraversal{stunServers: []string{}}

	ctx := context.Background()
	_, _, err := nat.DiscoverExternalEndpoint(ctx, 0)
	if err == nil {
		t.Error("expected error when no STUN servers configured")
	}
}

func TestNATTraversal_InvalidServer(t *testing.T) {
	nat := NewNATTraversal([]string{"127.0.0.1:1"}) // nothing listening

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := nat.DiscoverExternalEndpoint(ctx, 0)
	if err == nil {
		t.Error("expected error for unreachable STUN server")
	}
}

func TestNewNATTraversal_DefaultServers(t *testing.T) {
	nat := NewNATTraversal(nil)
	if len(nat.stunServers) != 1 {
		t.Fatalf("expected 1 default server, got %d", len(nat.stunServers))
	}
	if nat.stunServers[0] != "stun.l.google.com:19302" {
		t.Errorf("expected default server stun.l.google.com:19302, got %s", nat.stunServers[0])
	}
}

func TestNATType_String(t *testing.T) {
	tests := []struct {
		nt   NATType
		want string
	}{
		{NATTypeNone, "none"},
		{NATTypeFullCone, "full_cone"},
		{NATTypeRestricted, "restricted"},
		{NATTypeSymmetric, "symmetric"},
	}

	for _, tt := range tests {
		if string(tt.nt) != tt.want {
			t.Errorf("NATType %v: expected %q, got %q", tt.nt, tt.want, string(tt.nt))
		}
	}
}
