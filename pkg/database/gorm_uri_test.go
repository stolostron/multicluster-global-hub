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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompletePostgres verifies GORM URI parse failures do not leak credentials.
func TestCompletePostgres(t *testing.T) {
	t.Run("valid uri", func(t *testing.T) {
		got, err := completePostgres("postgres://user:secret@localhost:5432/hoh?sslmode=disable", "")
		require.NoError(t, err, "valid postgres URI must parse")
		require.NotNil(t, got, "parsed postgres URI must be returned")
		assert.Equal(t, "localhost:5432", got.Host, "parsed postgres host must match")
	})

	t.Run("malformed uri with percent-escaped password does not leak credentials", func(t *testing.T) {
		const sentinel = "secret-password"
		raw := "postgres://user:" + sentinel + "%zz@localhost:5432/hoh"

		_, err := completePostgres(raw, "")
		require.Error(t, err, "malformed postgres URI must be rejected")
		assert.ErrorIs(t, err, errParseDatabaseURI,
			"parse errors must use a fixed message")
		assert.NotContains(t, err.Error(), raw,
			"parse errors must not echo the raw connection URI")
		assert.NotContains(t, err.Error(), "postgres://",
			"parse errors must not echo the URI scheme")
		assert.NotContains(t, err.Error(), sentinel,
			"parse errors must not echo the postgres password")
	})

	t.Run("verify-full attaches CA without replacing sslmode", func(t *testing.T) {
		caPath := t.TempDir() + "/ca.crt"
		require.NoError(t, os.WriteFile(caPath, []byte("test-ca"), 0o600),
			"test CA file must be written")

		got, err := completePostgres(
			"postgres://user:secret@localhost:5432/hoh?sslmode=verify-full", caPath)
		require.NoError(t, err, "verify-full URI with a CA path must parse")
		require.NotNil(t, got, "parsed postgres URI must be returned")
		assert.Equal(t, "verify-full", got.Query().Get("sslmode"),
			"verify-full must be preserved when a CA is configured")
		assert.Equal(t, []string{"verify-full"}, got.Query()["sslmode"],
			"sslmode must not be duplicated with disable")
		assert.Equal(t, caPath, got.Query().Get("sslrootcert"),
			"verify-full must attach sslrootcert when a CA path is configured")
	})

	t.Run("require sslmode is preserved without a CA", func(t *testing.T) {
		got, err := completePostgres(
			"postgres://user:secret@localhost:5432/hoh?sslmode=require", "")
		require.NoError(t, err, "require URI without a CA must parse")
		require.NotNil(t, got, "parsed postgres URI must be returned")
		assert.Equal(t, "require", got.Query().Get("sslmode"),
			"explicit require must not be rewritten to disable")
		assert.Equal(t, []string{"require"}, got.Query()["sslmode"],
			"sslmode must not be duplicated with disable")
		assert.Empty(t, got.Query().Get("sslrootcert"),
			"require without a CA must not set sslrootcert")
	})

	t.Run("missing sslmode defaults to disable", func(t *testing.T) {
		got, err := completePostgres("postgres://user:secret@localhost:5432/hoh", "")
		require.NoError(t, err, "URI without sslmode must parse")
		require.NotNil(t, got, "parsed postgres URI must be returned")
		assert.Equal(t, "disable", got.Query().Get("sslmode"),
			"unset sslmode must default to disable")
	})
}
