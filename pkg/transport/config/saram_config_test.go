package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTLSConfigMismatchedClientCertAndKey(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	caFile := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(certFile, []byte("cert"), 0o600); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}
	if err := os.WriteFile(caFile, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"), 0o600); err != nil {
		t.Fatalf("failed to write ca file: %v", err)
	}

	_, err := NewTLSConfig(certFile, keyFile, caFile)
	if err == nil {
		t.Fatal("expected error when only client certificate is provided")
	}
}
