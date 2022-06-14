package compressor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
)

const (
	gzipCompressorErrorString = "gzip compressor error"
	gzipType                  = "gzip"
)

// newGZipCompressor returns a new instance of gzip-based compressor.
func newGZipCompressor() Compressor {
	return &CompressorGZip{}
}

// CompressorGZip implements Compressor with gzip-based logic.
type CompressorGZip struct{}

// GetType returns the string identifier for GZip compressor.
func (compressor *CompressorGZip) GetType() string {
	return gzipType
}

// Compress compresses a slice of bytes using gzip lib.
func (compressor *CompressorGZip) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, fmt.Errorf("%s - %w", gzipCompressorErrorString, err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("%s - %w", gzipCompressorErrorString, err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses a slice of gzip-compressed bytes using gzip lib.
func (compressor *CompressorGZip) Decompress(compressedData []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewBuffer(compressedData))
	if err != nil {
		return nil, fmt.Errorf("%s - %w", gzipCompressorErrorString, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("%s - %w", gzipCompressorErrorString, err)
	}

	if err = reader.Close(); err != nil {
		return nil, fmt.Errorf("%s - %w", gzipCompressorErrorString, err)
	}

	return data, nil
}
