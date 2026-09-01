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

package inventory

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F004: database credentials must not appear in error messages.
func TestPostgresPasswordFromURI(t *testing.T) {
	t.Run("valid uri", func(t *testing.T) {
		uri, err := url.Parse("postgresql://inventory:secret@postgres.example:5432/hoh")
		require.NoError(t, err, "test URI must parse for password extraction coverage")

		password, err := postgresPasswordFromURI(uri)
		require.NoError(t, err, "valid inventory URI must yield a postgres password")
		assert.Equal(t, "secret", password, "postgres password must match the URI credential")
	})

	t.Run("missing password does not leak connection string", func(t *testing.T) {
		uri, err := url.Parse("postgresql://inventory@postgres.example:5432/hoh")
		require.NoError(t, err, "test URI must parse for missing-password coverage")

		_, err = postgresPasswordFromURI(uri)
		require.Error(t, err, "inventory URI without a password must be rejected")
		assert.Equal(t, "postgres connection is missing a password", err.Error(),
			"missing-password errors must use a stable message")
		assert.NotContains(t, err.Error(), "postgresql://",
			"postgres errors must not echo the raw connection URI")
		assert.NotContains(t, err.Error(), "inventory@",
			"postgres errors must not echo the database username")
	})
}

// TestParsePostgresURI verifies URI parse failures do not leak credentials.
func TestParsePostgresURI(t *testing.T) {
	t.Run("valid uri", func(t *testing.T) {
		uri, err := parsePostgresURI("postgresql://inventory:secret@postgres.example:5432/hoh")
		require.NoError(t, err, "valid inventory URI must parse")
		require.NotNil(t, uri, "parsed inventory URI must be returned")
		assert.Equal(t, "postgres.example:5432", uri.Host,
			"parsed inventory URI host must match")
	})

	t.Run("malformed uri with percent-escaped password does not leak credentials", func(t *testing.T) {
		const sentinel = "secret-password"
		raw := "postgresql://inventory:" + sentinel + "%zz@postgres.example:5432/hoh"

		_, err := parsePostgresURI(raw)
		require.Error(t, err, "malformed inventory URI must be rejected")
		assert.Equal(t, "failed to parse postgres connection", err.Error(),
			"parse errors must use a fixed message")
		assert.NotContains(t, err.Error(), raw,
			"parse errors must not echo the raw connection URI")
		assert.NotContains(t, err.Error(), "postgresql://",
			"parse errors must not echo the URI scheme")
		assert.NotContains(t, err.Error(), sentinel,
			"parse errors must not echo the postgres password")
	})
}
