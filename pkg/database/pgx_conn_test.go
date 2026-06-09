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

const testPostgresURIRequireSSL = "postgres://user:pass@localhost:5432/db?sslmode=require"

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

	parsed, err := pgx.ParseConfig(testPostgresURI)
	if err != nil {
		t.Fatalf("failed to parse uri: %v", err)
	}
	if parsed.TLSConfig != nil {
		t.Skip("pgx sets TLSConfig for this URI; MinVersion branch requires nil TLSConfig")
	}

	cfg, err := GetPostgresConfig(testPostgresURI, caPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("expected TLSConfig to be set when CA is provided")
	}
	if cfg.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", cfg.TLSConfig.MinVersion)
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

	baseline, err := pgx.ParseConfig(testPostgresURIRequireSSL)
	if err != nil {
		t.Fatalf("failed to parse uri: %v", err)
	}
	if baseline.TLSConfig == nil {
		t.Fatal("expected pgx to set TLSConfig for sslmode=require")
	}
	expectedServerName := baseline.TLSConfig.ServerName
	expectedMinVersion := baseline.TLSConfig.MinVersion

	cfg, err := GetPostgresConfig(testPostgresURIRequireSSL, caPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.RootCAs == nil {
		t.Fatal("expected TLSConfig with RootCAs")
	}
	if cfg.TLSConfig.ServerName != expectedServerName {
		t.Fatalf("expected ServerName %q, got %q", expectedServerName, cfg.TLSConfig.ServerName)
	}
	if cfg.TLSConfig.MinVersion != expectedMinVersion {
		t.Fatalf("expected MinVersion %d, got %d", expectedMinVersion, cfg.TLSConfig.MinVersion)
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
