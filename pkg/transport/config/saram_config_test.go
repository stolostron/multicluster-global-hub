package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestNewTLSConfigMismatchedClientCertAndKey(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	caFile := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(certFile, []byte("cert"), 0o600); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("failed to write ca file: %v", err)
	}

	_, err := NewTLSConfig(certFile, keyFile, caFile)
	if err == nil {
		t.Fatal("expected error when only client certificate is provided")
	}
	if !strings.Contains(err.Error(), errClientCertKeyMismatch) {
		t.Fatalf("expected error %q, got %v", errClientCertKeyMismatch, err)
	}
}

func TestNewTLSConfigMismatchedKeyOnly(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "tls.key")
	caFile := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("failed to write ca file: %v", err)
	}

	_, err := NewTLSConfig("", keyFile, caFile)
	if err == nil {
		t.Fatal("expected error when only client key is provided")
	}
	if !strings.Contains(err.Error(), errClientCertKeyMismatch) {
		t.Fatalf("expected error %q, got %v", errClientCertKeyMismatch, err)
	}
}

func TestNewTLSConfigWithClientCertAndCA(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, caPEM := generateTestCertKeyAndCA(t)

	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	caFile := filepath.Join(dir, "ca.crt")
	writeFile(t, certFile, certPEM)
	writeFile(t, keyFile, keyPEM)
	writeFile(t, caFile, caPEM)

	cfg, err := NewTLSConfig(certFile, keyFile, caFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected one client certificate, got %d", len(cfg.Certificates))
	}
	if cfg.RootCAs == nil {
		t.Fatal("expected RootCAs to be set")
	}
}

func TestGetSaramaConfigWithoutTLS(t *testing.T) {
	cfg, err := GetSaramaConfig(&transport.KafkaInternalConfig{EnableTLS: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Net.TLS.Enable {
		t.Fatal("expected TLS to be disabled")
	}
}

func generateTestCertKeyAndCA(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create client cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, caPEM
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
