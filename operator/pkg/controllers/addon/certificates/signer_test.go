package certificates

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	certutil "k8s.io/client-go/util/cert"
)

func TestCSRSigner(t *testing.T) {
	caCert, cakey, err := generateKeyAndCert()
	assert.Nil(t, err)

	csr := newCSR("test", "cluster1")
	// sign the cert
	certBytes := Sign(csr, cakey, caCert)
	assert.NotNil(t, certBytes, "expect cert not be nil")

	// parse cert
	certs, err := certutil.ParseCertsPEM(certBytes)
	assert.Nil(t, err, "failed to parse cert")
	assert.Equal(t, 1, len(certs))

	// validate the CN
	assert.Equal(t, certs[0].Subject.CommonName, "test", "CN is not correct")
}

func generateKeyAndCert() ([]byte, []byte, error) {
	// Generate the RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("rsa key failed to generate: %v", err)
	}

	// Create a self-signed certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %v", err)
	}

	certTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "test-cn",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // Valid for 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Self-sign the certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create self-signed certificate: %v", err)
	}

	// Encode the private key to PEM format
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Encode the certificate to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	return certPEM, keyPEM, nil
}
