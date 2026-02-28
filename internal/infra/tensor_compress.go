package infra

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// TensorCompressor handles zstd compression/decompression for tensor transfers.
// zstd provides ~40% size reduction on float tensor data with minimal latency impact.
type TensorCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewTensorCompressor creates a compressor with the specified compression level.
// Level 3 (default) balances speed and compression ratio for real-time tensor transfer.
func NewTensorCompressor() (*TensorCompressor, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedDefault),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	return &TensorCompressor{
		encoder: encoder,
		decoder: decoder,
	}, nil
}

// Compress compresses tensor data using zstd.
// Returns compressed bytes and the compression ratio (compressed/original).
func (tc *TensorCompressor) Compress(data []byte) ([]byte, float64) {
	if len(data) == 0 {
		return data, 1.0
	}

	compressed := tc.encoder.EncodeAll(data, make([]byte, 0, len(data)/2))
	ratio := float64(len(compressed)) / float64(len(data))
	return compressed, ratio
}

// Decompress decompresses zstd-compressed tensor data.
func (tc *TensorCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	decompressed, err := tc.decoder.DecodeAll(data, make([]byte, 0, len(data)*2))
	if err != nil {
		return nil, fmt.Errorf("zstd decompression failed: %w", err)
	}

	return decompressed, nil
}

// CompressStream compresses data from a reader and writes to a writer.
// Useful for large tensor transfers to avoid loading entire tensor in memory.
func (tc *TensorCompressor) CompressStream(dst io.Writer, src io.Reader) (int64, error) {
	enc, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return 0, fmt.Errorf("failed to create stream encoder: %w", err)
	}
	defer enc.Close()

	n, err := io.Copy(enc, src)
	if err != nil {
		return n, fmt.Errorf("stream compression failed: %w", err)
	}

	if err := enc.Close(); err != nil {
		return n, fmt.Errorf("failed to flush stream encoder: %w", err)
	}

	return n, nil
}

// DecompressStream decompresses data from a reader and writes to a writer.
func (tc *TensorCompressor) DecompressStream(dst io.Writer, src io.Reader) (int64, error) {
	dec, err := zstd.NewReader(src)
	if err != nil {
		return 0, fmt.Errorf("failed to create stream decoder: %w", err)
	}
	defer dec.Close()

	n, err := io.Copy(dst, dec)
	if err != nil {
		return n, fmt.Errorf("stream decompression failed: %w", err)
	}

	return n, nil
}

// ShouldCompress returns true if the data is large enough to benefit from compression.
// Below 1KB the overhead of compression headers outweighs the size savings.
func ShouldCompress(data []byte) bool {
	return len(data) > 1024
}

// CompressIfBeneficial compresses data only if it's large enough.
// Returns the data (possibly compressed), whether it was compressed, and the ratio.
func (tc *TensorCompressor) CompressIfBeneficial(data []byte) ([]byte, bool, float64) {
	if !ShouldCompress(data) {
		return data, false, 1.0
	}

	compressed, ratio := tc.Compress(data)

	// Only use compressed data if it's actually smaller
	if ratio >= 1.0 {
		return data, false, 1.0
	}

	return compressed, true, ratio
}

// Close releases encoder and decoder resources.
func (tc *TensorCompressor) Close() {
	tc.encoder.Close()
	tc.decoder.Close()
}

// RoundTripVerify compresses and decompresses data, verifying integrity.
// Used for testing and initial validation.
func (tc *TensorCompressor) RoundTripVerify(data []byte) error {
	compressed, _ := tc.Compress(data)
	decompressed, err := tc.Decompress(compressed)
	if err != nil {
		return fmt.Errorf("round-trip decompression failed: %w", err)
	}

	if !bytes.Equal(data, decompressed) {
		return fmt.Errorf("round-trip data mismatch: original %d bytes, decompressed %d bytes",
			len(data), len(decompressed))
	}

	return nil
}
