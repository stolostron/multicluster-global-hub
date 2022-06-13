package compressor

import (
	"errors"
)

// Compressor declares the functionality provided by the different supported compressors.
type Compressor interface {
	// GetType returns the string type identifier of the compressor.
	GetType() string
	// Compress compresses a slice of bytes.
	Compress([]byte) ([]byte, error)
	// Decompress decompresses a slice of compressed bytes.
	Decompress([]byte) ([]byte, error)
}

var errCompressionTypeNotFound = errors.New("compression type not supported")

// CompressionType is the type identifying supported compression methods.
type CompressionType string

const (
	// NoOp is used to create a no-op Compressor.
	NoOp CompressionType = "no-op"
	// GZip is used to create a gzip-based Compressor.
	GZip CompressionType = "gzip"
)

// NewCompressor returns a compressor instance that corresponds to the given CompressionType.
func NewCompressor(compressionType CompressionType) (Compressor, error) {
	switch compressionType {
	case NoOp:
		return newNoOpCompressor(), nil
	case GZip:
		return newGZipCompressor(), nil
	default:
		return nil, errCompressionTypeNotFound
	}
}
