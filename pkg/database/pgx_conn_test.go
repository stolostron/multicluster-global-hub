package database

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPostgresConfig_CA(t *testing.T) {
	uri := "postgres://user:pass@localhost:5432/hoh?sslmode=require"

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "postgres-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	cfg, err := GetPostgresConfig(uri, caPEM)
	require.NoError(t, err)
	require.NotNil(t, cfg.TLSConfig)
	assert.NotNil(t, cfg.TLSConfig.RootCAs)

	_, err = GetPostgresConfig(uri, []byte("not-a-pem"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse postgres CA certificate")
}
