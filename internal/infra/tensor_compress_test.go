package infra

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestTensorCompressor_RoundTrip(t *testing.T) {
	tc, err := NewTensorCompressor()
	if err != nil {
		t.Fatalf("failed to create compressor: %v", err)
	}
	defer tc.Close()

	// Simulate float32 tensor data (repetitive patterns compress well)
	data := []byte(strings.Repeat("ABCDEFGHIJKLMNOP", 1024)) // 16KB

	compressed, ratio := tc.Compress(data)
	if ratio >= 1.0 {
		t.Errorf("expected compression ratio < 1.0, got %f", ratio)
	}

	if len(compressed) >= len(data) {
		t.Errorf("compressed size (%d) should be less than original (%d)", len(compressed), len(data))
	}

	decompressed, err := tc.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Error("decompressed data does not match original")
	}
}

func TestTensorCompressor_EmptyData(t *testing.T) {
	tc, err := NewTensorCompressor()
	if err != nil {
		t.Fatalf("failed to create compressor: %v", err)
	}
	defer tc.Close()

	compressed, ratio := tc.Compress(nil)
	if ratio != 1.0 {
		t.Errorf("expected ratio 1.0 for empty data, got %f", ratio)
	}
	if len(compressed) != 0 {
		t.Errorf("expected empty compressed for nil, got %d bytes", len(compressed))
	}

	decompressed, err := tc.Decompress(nil)
	if err != nil {
		t.Fatalf("decompression of nil failed: %v", err)
	}
	if len(decompressed) != 0 {
		t.Errorf("expected empty decompressed for nil, got %d bytes", len(decompressed))
	}
}

func TestTensorCompressor_RandomData(t *testing.T) {
	tc, err := NewTensorCompressor()
	if err != nil {
		t.Fatalf("failed to create compressor: %v", err)
	}
	defer tc.Close()

	// Random data is hard to compress
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	if err := tc.RoundTripVerify(data); err != nil {
		t.Errorf("round trip verification failed: %v", err)
	}
}

func TestTensorCompressor_CompressIfBeneficial(t *testing.T) {
	tc, err := NewTensorCompressor()
	if err != nil {
		t.Fatalf("failed to create compressor: %v", err)
	}
	defer tc.Close()

	// Small data (< 1KB) should not be compressed
	smallData := []byte("hello world")
	result, compressed, _ := tc.CompressIfBeneficial(smallData)
	if compressed {
		t.Error("small data should not be compressed")
	}
	if !bytes.Equal(result, smallData) {
		t.Error("small data should be returned as-is")
	}

	// Large repetitive data should be compressed
	largeData := []byte(strings.Repeat("tensor_float32_", 256)) // ~3.8KB
	result, compressed, ratio := tc.CompressIfBeneficial(largeData)
	if !compressed {
		t.Error("large repetitive data should be compressed")
	}
	if ratio >= 1.0 {
		t.Errorf("expected beneficial ratio < 1.0, got %f", ratio)
	}
	if len(result) >= len(largeData) {
		t.Errorf("compressed size (%d) should be less than original (%d)", len(result), len(largeData))
	}
}

func TestShouldCompress(t *testing.T) {
	if ShouldCompress(make([]byte, 100)) {
		t.Error("100 bytes should not trigger compression")
	}
	if ShouldCompress(make([]byte, 1024)) {
		t.Error("exactly 1024 bytes should not trigger compression")
	}
	if !ShouldCompress(make([]byte, 1025)) {
		t.Error("1025 bytes should trigger compression")
	}
}

func TestTensorCompressor_Stream(t *testing.T) {
	tc, err := NewTensorCompressor()
	if err != nil {
		t.Fatalf("failed to create compressor: %v", err)
	}
	defer tc.Close()

	original := []byte(strings.Repeat("stream_tensor_data_", 512)) // ~9.5KB

	// Compress via stream
	var compressedBuf bytes.Buffer
	_, err = tc.CompressStream(&compressedBuf, bytes.NewReader(original))
	if err != nil {
		t.Fatalf("stream compression failed: %v", err)
	}

	// Decompress via stream
	var decompressedBuf bytes.Buffer
	_, err = tc.DecompressStream(&decompressedBuf, &compressedBuf)
	if err != nil {
		t.Fatalf("stream decompression failed: %v", err)
	}

	if !bytes.Equal(original, decompressedBuf.Bytes()) {
		t.Error("stream round-trip data mismatch")
	}
}
