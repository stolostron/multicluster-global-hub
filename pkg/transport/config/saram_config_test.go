// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type tlsTestMaterials struct {
	certFile string
	keyFile  string
	caFile   string
}

// writeTLSTestMaterials writes temporary client and CA certificates for TLS tests.
func writeTLSTestMaterials(t *testing.T) tlsTestMaterials {
	t.Helper()
	dir := t.TempDir()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "test CA key generation must succeed")
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err, "test CA certificate creation must succeed")

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "test client key generation must succeed")
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "test-client"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caTmpl, &clientKey.PublicKey, caKey)
	require.NoError(t, err, "test client certificate creation must succeed")

	certFile := filepath.Join(dir, "client.crt")
	keyFile := filepath.Join(dir, "client.key")
	caFile := filepath.Join(dir, "ca.crt")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
	})
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600), "client cert file must be written for mTLS test")
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600), "client key file must be written for mTLS test")
	require.NoError(t, os.WriteFile(caFile, caPEM, 0o600), "CA file must be written for broker verification test")

	return tlsTestMaterials{certFile: certFile, keyFile: keyFile, caFile: caFile}
}

// F002: Kafka TLS must verify server certificates and require client credentials.
// TestNewTLSConfig verifies Kafka TLS requires matching client credentials and a valid CA.
func TestNewTLSConfig(t *testing.T) {
	materials := writeTLSTestMaterials(t)

	tests := []struct {
		name        string
		certFile    string
		keyFile     string
		caFile      string
		wantErr     bool
		errContains string
		errIs       error
	}{
		{
			name:     "valid mTLS config",
			certFile: materials.certFile,
			keyFile:  materials.keyFile,
			caFile:   materials.caFile,
		},
		{
			name:    "missing client credentials",
			caFile:  materials.caFile,
			wantErr: true,
			errIs:   errTLSClientCredentialsRequired,
		},
		{
			name:        "cert without key",
			certFile:    materials.certFile,
			caFile:      materials.caFile,
			wantErr:     true,
			errContains: errClientCertKeyMismatch,
		},
		{
			name:        "missing CA file",
			certFile:    materials.certFile,
			keyFile:     materials.keyFile,
			caFile:      filepath.Join(t.TempDir(), "missing-ca.crt"),
			wantErr:     true,
			errContains: "failed to read CA certificate",
		},
		{
			name:        "invalid CA PEM",
			certFile:    materials.certFile,
			keyFile:     materials.keyFile,
			caFile:      filepath.Join(t.TempDir(), "bad-ca.pem"),
			wantErr:     true,
			errContains: "failed to parse CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.caFile != "" && tt.errContains == "failed to parse CA certificate" {
				require.NoError(t, os.WriteFile(tt.caFile, []byte("not-a-pem"), 0o600),
					"invalid CA fixture must be written for parse-failure test")
			}
			cfg, err := NewTLSConfig(tt.certFile, tt.keyFile, tt.caFile)
			if tt.wantErr {
				require.Error(t, err, "TLS config must fail for invalid credentials or CA material")
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs, "error should match expected TLS credential failure")
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"error should describe the TLS validation failure")
				}
				return
			}
			require.NoError(t, err, "valid mTLS materials must produce a TLS config")
			assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion,
				"Kafka TLS must enforce TLS 1.2 minimum")
			assert.NotNil(t, cfg.RootCAs, "broker CA pool must be configured for verification")
			assert.Len(t, cfg.Certificates, 1, "client certificate must be loaded for mTLS")
			assert.False(t, cfg.InsecureSkipVerify,
				"InsecureSkipVerify must remain disabled for Kafka connections")
		})
	}
}

// TestGetSaramaConfig_TLS verifies Sarama TLS is enabled when valid credentials are provided.
func TestGetSaramaConfig_TLS(t *testing.T) {
	materials := writeTLSTestMaterials(t)

	cfg, err := GetSaramaConfig(&transport.KafkaInternalConfig{
		EnableTLS:      true,
		ClientCertPath: materials.certFile,
		ClientKeyPath:  materials.keyFile,
		CaCertPath:     materials.caFile,
	})
	require.NoError(t, err, "Sarama TLS config must be created from valid mTLS materials")
	require.True(t, cfg.Net.TLS.Enable, "TLS must be enabled when Kafka TLS is requested")
	require.NotNil(t, cfg.Net.TLS.Config, "TLS config must be attached to Sarama client settings")
	assert.False(t, cfg.Net.TLS.Config.InsecureSkipVerify,
		"Sarama client must verify broker certificates")
}

// TestGetSaramaConfig_TLSRequiresValidCA verifies TLS configuration fails closed without a CA.
func TestGetSaramaConfig_TLSRequiresValidCA(t *testing.T) {
	materials := writeTLSTestMaterials(t)

	_, err := GetSaramaConfig(&transport.KafkaInternalConfig{
		EnableTLS:      true,
		ClientCertPath: materials.certFile,
		ClientKeyPath:  materials.keyFile,
		CaCertPath:     filepath.Join(t.TempDir(), "missing-ca.crt"),
	})
	require.Error(t, err, "Kafka TLS must fail closed when the CA certificate is missing")
	assert.Contains(t, err.Error(), "failed to read CA certificate",
		"missing CA errors must describe the broker verification failure")
}
