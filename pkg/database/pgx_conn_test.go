package database

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const testPostgresURI = "postgres://user:pass@localhost:5432/db"

func TestGetPostgresConfigWithoutCert(t *testing.T) {
	cfg, err := GetPostgresConfig(testPostgresURI, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig != nil && cfg.TLSConfig.RootCAs != nil {
		t.Fatal("expected no custom RootCAs without cert")
	}
}

func TestGetPostgresConfigWithValidCA(t *testing.T) {
	caPEM := generateTestCAPEM(t)

	cfg, err := GetPostgresConfig(testPostgresURI, caPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("expected TLSConfig to be set")
	}
	if cfg.TLSConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be set")
	}
}

func TestGetPostgresConfigSetsMinVersionWhenTLSUnset(t *testing.T) {
	caPEM := generateTestCAPEM(t)
	config, err := pgx.ParseConfig(testPostgresURI)
	if err != nil {
		t.Fatalf("failed to parse uri: %v", err)
	}
	config.TLSConfig = nil

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to parse generated CA")
	}
	config.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	config.TLSConfig.RootCAs = caCertPool

	if config.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", config.TLSConfig.MinVersion)
	}
}

func TestGetPostgresConfigWithInvalidCA(t *testing.T) {
	_, err := GetPostgresConfig(testPostgresURI, []byte("not-a-pem-cert"))
	if err == nil {
		t.Fatal("expected error for invalid CA PEM")
	}
	if !strings.Contains(err.Error(), "failed to parse database CA certificate PEM") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPostgresConfigPreservesExistingTLSConfig(t *testing.T) {
	caPEM := generateTestCAPEM(t)

	config, err := pgx.ParseConfig(testPostgresURI)
	if err != nil {
		t.Fatalf("failed to parse uri: %v", err)
	}
	config.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: "db.example.com",
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to parse generated CA")
	}
	config.TLSConfig.RootCAs = caCertPool

	if config.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected preserved MinVersion TLS 1.3, got %d", config.TLSConfig.MinVersion)
	}
	if config.TLSConfig.ServerName != "db.example.com" {
		t.Fatalf("expected preserved ServerName, got %q", config.TLSConfig.ServerName)
	}
}

func TestGetPostgresConfigInvalidURI(t *testing.T) {
	_, err := GetPostgresConfig("not-a-valid-uri", nil)
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
	if !strings.Contains(err.Error(), "failed to parse database uri") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func generateTestCAPEM(t *testing.T) []byte {
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
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}
