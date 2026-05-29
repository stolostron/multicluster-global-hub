package requester

import (
	"context"
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

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestRefreshClientSetsTLS12MinVersion(t *testing.T) {
	certPEM, keyPEM, caPEM := generateInventoryTestCerts(t)
	restfulConn := &transport.RestfulConfig{
		Host:       "https://127.0.0.1:65530",
		ClientCert: string(certPEM),
		ClientKey:  string(keyPEM),
		CACert:     string(caPEM),
	}

	client := &InventoryClient{}
	if err := client.RefreshClient(context.Background(), restfulConn); err != nil {
		t.Fatalf("unexpected error building inventory client: %v", err)
	}
	if client.tlsConfig == nil {
		t.Fatal("expected tls config to be set before client init failure")
	}
	if client.tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", client.tlsConfig.MinVersion)
	}
}

func TestRefreshClientInvalidClientCert(t *testing.T) {
	_, keyPEM, caPEM := generateInventoryTestCerts(t)
	restfulConn := &transport.RestfulConfig{
		Host:       "https://127.0.0.1:65530",
		ClientCert: "invalid-cert",
		ClientKey:  string(keyPEM),
		CACert:     string(caPEM),
	}

	client := &InventoryClient{}
	err := client.RefreshClient(context.Background(), restfulConn)
	if err == nil {
		t.Fatal("expected error for invalid client cert")
	}
	if !strings.Contains(err.Error(), "failed the load client cert") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func generateInventoryTestCerts(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "inventory-test-ca"},
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
		Subject:      pkix.Name{CommonName: "inventory-test-client"},
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
