package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	errParseDatabaseURI           = errors.New("failed to parse database uri")
	errPostgresCARequiresVerify   = errors.New("postgres CA requires sslmode verify-ca or verify-full")
	errParsePostgresCACertificate = errors.New("failed to parse postgres CA certificate")
)

// PostgresConnection opens a PostgreSQL connection using the given URI and optional CA certificate.
func PostgresConnection(ctx context.Context, URI string, cert []byte) (*pgx.Conn, error) {
	return PostgresDBConn(ctx, URI, cert, "")
}

// PostgresDBConn opens a PostgreSQL connection, optionally overriding the database name.
func PostgresDBConn(ctx context.Context, URI string, cert []byte, db string) (*pgx.Conn, error) {
	config, err := GetPostgresConfig(URI, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to get database config: %w", err)
	}

	if db != "" {
		config.Database = db
	}

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}

// GetPostgresConfig parses a PostgreSQL URI and installs a CA pool when cert is provided.
// Parser errors use a fixed message so the URI cannot leak credentials.
func GetPostgresConfig(URI string, cert []byte) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(URI)
	if err != nil {
		return nil, errParseDatabaseURI
	}
	if len(cert) > 0 {
		if err := requireVerifyingPostgresSSLMode(postgresSSLModeFromURI(URI)); err != nil {
			return nil, fmt.Errorf("failed to validate postgres sslmode: %w", err)
		}
		if err := applyPostgresCA(config, cert); err != nil {
			return nil, fmt.Errorf("failed to apply postgres CA certificate: %w", err)
		}
	}
	return config, nil
}

// postgresPoolConfig parses pool settings and CA material without opening a connection.
func postgresPoolConfig(databaseURI string, certPath string, size int32) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(databaseURI)
	if err != nil {
		return nil, errParseDatabaseURI
	}

	if certPath != "" {
		if _, ok := utils.Validate(certPath); !ok {
			return nil, fmt.Errorf("invalid database cert file path")
		}
		cert, err := os.ReadFile(certPath) // #nosec G304
		if err != nil {
			return nil, fmt.Errorf("unable to read database cert file: %w", err)
		}
		if err := requireVerifyingPostgresSSLMode(postgresSSLModeFromURI(databaseURI)); err != nil {
			return nil, fmt.Errorf("failed to validate postgres sslmode: %w", err)
		}
		if err := applyPostgresCA(config.ConnConfig, cert); err != nil {
			return nil, fmt.Errorf("failed to apply postgres CA certificate: %w", err)
		}
	}

	if size > 0 {
		config.MaxConns = size
	}

	return config, nil
}

// PostgresConnPool returns a new postgres connection pool. size < 0 means default size.
func PostgresConnPool(ctx context.Context, databaseURI string, certPath string, size int32) (*pgxpool.Pool, error) {
	config, err := postgresPoolConfig(databaseURI, certPath, size)
	if err != nil {
		return nil, fmt.Errorf("failed to configure postgres connection pool: %w", err)
	}

	dbConnectionPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return dbConnectionPool, nil
}

// postgresSSLModeFromURI returns the URI sslmode, defaulting to prefer when unset.
func postgresSSLModeFromURI(databaseURI string) string {
	parsed, err := url.Parse(databaseURI)
	if err != nil {
		return "prefer"
	}
	if mode := parsed.Query().Get("sslmode"); mode != "" {
		return mode
	}
	return "prefer"
}

// requireVerifyingPostgresSSLMode rejects CA-backed connections that can skip verify or fall back to plaintext.
func requireVerifyingPostgresSSLMode(sslmode string) error {
	switch sslmode {
	case "verify-ca", "verify-full":
		return nil
	default:
		return errPostgresCARequiresVerify
	}
}

// applyPostgresCA installs the CA on the primary TLS config and every pgx fallback.
func applyPostgresCA(config *pgx.ConnConfig, cert []byte) error {
	updated, err := postgresTLSConfigWithCA(config.TLSConfig, cert)
	if err != nil {
		return fmt.Errorf("failed to apply postgres CA to connection config: %w", err)
	}
	config.TLSConfig = updated
	for _, fallback := range config.Fallbacks {
		if fallback == nil {
			continue
		}
		fallback.TLSConfig, err = postgresTLSConfigWithCA(fallback.TLSConfig, cert)
		if err != nil {
			return fmt.Errorf("failed to apply postgres CA to fallback connection config: %w", err)
		}
	}
	return nil
}

// postgresTLSConfigWithCA installs RootCAs on the parsed TLS config, preserving ServerName and related settings.
func postgresTLSConfigWithCA(existing *tls.Config, cert []byte) (*tls.Config, error) {
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(cert) {
		return nil, errParsePostgresCACertificate
	}
	if existing == nil {
		existing = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	existing.RootCAs = caCertPool
	return existing, nil
}
