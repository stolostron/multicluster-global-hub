package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const errMessageFileNotFound = "no such file or directory"

func PostgresConnection(ctx context.Context, URI string, cert []byte) (*pgx.Conn, error) {
	config, err := GetPostgresConfig(URI, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to get database config: %w", err)
	}

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}

func GetPostgresConfig(URI string, cert []byte) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database uri: %w", err)
	}
	if len(cert) > 0 { // #nosec G402
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(cert)
		config.TLSConfig = &tls.Config{
			RootCAs: caCertPool,
			// nolint:gosec
			InsecureSkipVerify: true, // #nosec G402
		}
	}
	return config, nil
}

// PostgresConnPool returns a new postgres connection pool. size < 0 means deafult size.
func PostgresConnPool(ctx context.Context, databaseURI string, certPath string, size int32) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURI)
	if err != nil {
		return nil, fmt.Errorf("unable to get postgres pool config: %w", err)
	}

	cert, err := os.ReadFile(certPath) // #nosec G304
	if err != nil && !strings.Contains(err.Error(), errMessageFileNotFound) {
		return nil, fmt.Errorf("unable to read database cert file: %w", err)
	}
	if len(cert) > 0 { // #nosec G402
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(cert)
		config.ConnConfig.TLSConfig = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: true, // #nosec G402
		}
	}

	if size > 0 {
		config.MaxConns = size
	}

	dbConnectionPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return dbConnectionPool, nil
}
