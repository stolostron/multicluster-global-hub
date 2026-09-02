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

package database

import (
	"crypto/rand"
	"crypto/rsa"
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
)

// testCAPEM returns a self-signed CA PEM used by PostgreSQL TLS unit tests.
func testCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "test CA key generation must succeed")
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "postgres-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err, "test CA certificate creation must succeed")
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// TestGetPostgresConfig verifies URI parsing, CA installation, and sslmode enforcement.
func TestGetPostgresConfig(t *testing.T) {
	uri := "postgres://user:pass@localhost:5432/hoh?sslmode=verify-full"
	caPEM := testCAPEM(t)

	t.Run("without cert leaves pgx defaults unchanged", func(t *testing.T) {
		cfg, err := GetPostgresConfig("postgres://user:pass@localhost:5432/hoh?sslmode=require", nil)
		require.NoError(t, err, "postgres config without a CA must parse")
		if cfg.TLSConfig != nil {
			assert.Nil(t, cfg.TLSConfig.RootCAs, "postgres config without CA must not install RootCAs")
		}
	})

	t.Run("with valid CA", func(t *testing.T) {
		cfg, err := GetPostgresConfig(uri, caPEM)
		require.NoError(t, err, "valid CA PEM must produce a postgres config")
		require.NotNil(t, cfg.TLSConfig, "CA-backed postgres config must set TLSConfig")
		assert.NotNil(t, cfg.TLSConfig.RootCAs, "CA-backed postgres config must install RootCAs")
	})

	t.Run("CA is installed on multi-host fallbacks", func(t *testing.T) {
		multi := "postgres://user:pass@primary:5432,standby:5432/hoh?sslmode=verify-full"
		cfg, err := GetPostgresConfig(multi, caPEM)
		require.NoError(t, err, "multi-host URI with CA must parse")
		require.NotNil(t, cfg.TLSConfig, "primary TLS config must be set")
		assert.NotNil(t, cfg.TLSConfig.RootCAs, "primary TLS config must install RootCAs")
		require.NotEmpty(t, cfg.Fallbacks, "multi-host URI must produce fallbacks")
		for i, fb := range cfg.Fallbacks {
			require.NotNil(t, fb, "fallback %d must not be nil", i)
			require.NotNil(t, fb.TLSConfig, "fallback %d must have TLSConfig", i)
			assert.NotNil(t, fb.TLSConfig.RootCAs, "fallback %d must install RootCAs", i)
		}
	})

	t.Run("invalid CA PEM", func(t *testing.T) {
		_, err := GetPostgresConfig(uri, []byte("not-a-pem"))
		require.Error(t, err, "invalid CA PEM must be rejected")
		assert.ErrorIs(t, err, errParsePostgresCACertificate,
			"error should identify CA parsing failure")
	})

	t.Run("sslmode require with CA is rejected", func(t *testing.T) {
		_, err := GetPostgresConfig("postgres://user:pass@localhost:5432/hoh?sslmode=require", caPEM)
		require.Error(t, err, "CA-backed sslmode=require must be rejected")
		assert.ErrorIs(t, err, errPostgresCARequiresVerify,
			"CA-backed connections must require verify-ca or verify-full")
	})

	t.Run("sslmode prefer with CA is rejected", func(t *testing.T) {
		_, err := GetPostgresConfig("postgres://user:pass@localhost:5432/hoh?sslmode=prefer", caPEM)
		require.Error(t, err, "CA-backed sslmode=prefer must be rejected")
		assert.ErrorIs(t, err, errPostgresCARequiresVerify,
			"CA-backed connections must require verify-ca or verify-full")
	})

	t.Run("invalid URI", func(t *testing.T) {
		_, err := GetPostgresConfig("://bad-uri", caPEM)
		require.Error(t, err, "invalid postgres URI must be rejected")
		assert.ErrorIs(t, err, errParseDatabaseURI,
			"URI parse errors must use a fixed message")
	})

	t.Run("invalid URI does not leak password", func(t *testing.T) {
		const sentinel = "secret-password"
		_, err := GetPostgresConfig("postgres://user:"+sentinel+"%zz@localhost:5432/hoh", nil)
		require.Error(t, err, "malformed postgres URI must be rejected")
		assert.ErrorIs(t, err, errParseDatabaseURI,
			"URI parse errors must use a fixed message")
		assert.NotContains(t, err.Error(), sentinel,
			"URI parse errors must not expose the postgres password")
		assert.NotContains(t, err.Error(), "postgres://",
			"URI parse errors must not expose the connection string")
	})
}

// TestPostgresPoolConfig_CA verifies CA file handling and sslmode checks without opening a pool.
func TestPostgresPoolConfig_CA(t *testing.T) {
	uri := "postgres://user:pass@localhost:5432/hoh?sslmode=verify-full"
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caPath, testCAPEM(t), 0o600), "test CA file must be written")

	t.Run("with valid CA file", func(t *testing.T) {
		cfg, err := postgresPoolConfig(uri, caPath, 1)
		require.NoError(t, err, "valid CA file must produce a pool config")
		require.NotNil(t, cfg.ConnConfig.TLSConfig, "CA-backed pool config must set TLSConfig")
		assert.NotNil(t, cfg.ConnConfig.TLSConfig.RootCAs, "CA-backed pool config must install RootCAs")
		assert.Equal(t, int32(1), cfg.MaxConns, "requested pool size must be applied")
	})

	t.Run("CA is installed on multi-host fallbacks", func(t *testing.T) {
		multi := "postgres://user:pass@primary:5432,standby:5432/hoh?sslmode=verify-full"
		cfg, err := postgresPoolConfig(multi, caPath, 1)
		require.NoError(t, err, "multi-host URI with CA must produce a pool config")
		require.NotNil(t, cfg.ConnConfig.TLSConfig, "primary TLS config must be set")
		assert.NotNil(t, cfg.ConnConfig.TLSConfig.RootCAs, "primary TLS config must install RootCAs")
		require.NotEmpty(t, cfg.ConnConfig.Fallbacks, "multi-host URI must produce fallbacks")
		for i, fb := range cfg.ConnConfig.Fallbacks {
			require.NotNil(t, fb, "fallback %d must not be nil", i)
			require.NotNil(t, fb.TLSConfig, "fallback %d must have TLSConfig", i)
			assert.NotNil(t, fb.TLSConfig.RootCAs, "fallback %d must install RootCAs", i)
		}
	})

	t.Run("invalid CA PEM", func(t *testing.T) {
		badPath := filepath.Join(dir, "bad-ca.crt")
		require.NoError(t, os.WriteFile(badPath, []byte("not-a-pem"), 0o600),
			"invalid CA file must be written for coverage")
		_, err := postgresPoolConfig(uri, badPath, 1)
		require.Error(t, err, "invalid CA PEM must be rejected")
		assert.ErrorIs(t, err, errParsePostgresCACertificate,
			"error should identify CA parsing failure")
	})

	t.Run("missing configured cert file is rejected", func(t *testing.T) {
		_, err := postgresPoolConfig(uri, filepath.Join(dir, "missing.crt"), 1)
		require.Error(t, err, "a configured CA path that does not exist must fail closed")
		assert.Contains(t, err.Error(), "unable to read database cert file",
			"missing CA path errors must identify the cert-file read")
	})

	t.Run("empty cert path skips CA", func(t *testing.T) {
		cfg, err := postgresPoolConfig(uri, "", 1)
		require.NoError(t, err, "empty CA path should skip cert loading")
		if cfg.ConnConfig.TLSConfig != nil {
			assert.Nil(t, cfg.ConnConfig.TLSConfig.RootCAs,
				"empty CA path must not install RootCAs")
		}
	})

	t.Run("sslmode require with CA is rejected", func(t *testing.T) {
		_, err := postgresPoolConfig("postgres://user:pass@localhost:5432/hoh?sslmode=require", caPath, 1)
		require.Error(t, err, "CA-backed sslmode=require must be rejected")
		assert.ErrorIs(t, err, errPostgresCARequiresVerify,
			"CA-backed connections must require verify-ca or verify-full")
	})

	t.Run("sslmode prefer with CA is rejected", func(t *testing.T) {
		_, err := postgresPoolConfig("postgres://user:pass@localhost:5432/hoh?sslmode=prefer", caPath, 1)
		require.Error(t, err, "CA-backed sslmode=prefer must be rejected")
		assert.ErrorIs(t, err, errPostgresCARequiresVerify,
			"CA-backed connections must require verify-ca or verify-full")
	})

	t.Run("invalid URI does not leak password", func(t *testing.T) {
		const sentinel = "secret-password"
		_, err := postgresPoolConfig("postgres://user:"+sentinel+"%zz@localhost:5432/hoh", "", 1)
		require.Error(t, err, "malformed postgres URI must be rejected")
		assert.ErrorIs(t, err, errParseDatabaseURI,
			"URI parse errors must use a fixed message")
		assert.NotContains(t, err.Error(), sentinel,
			"URI parse errors must not expose the postgres password")
	})
}
