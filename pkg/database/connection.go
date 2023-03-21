package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
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
	if len(cert) > 0 {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(cert)
		/* #nosec G402*/
		config.TLSConfig = &tls.Config{
			RootCAs: caCertPool,
			//nolint:gosec
			InsecureSkipVerify: true,
		}
	}
	return config, nil
}

func PostgresConnPool(ctx context.Context, databaseURI string, certPath string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURI)
	if err != nil {
		return nil, fmt.Errorf("unable to get postgres pool config: %w", err)
	}

	cert, err := ioutil.ReadFile(certPath) // #nosec G304
	if err != nil && !strings.Contains(err.Error(), errMessageFileNotFound) {
		return nil, fmt.Errorf("unable to read database cert file: %w", err)
	}
	if len(cert) > 0 {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(cert)
		/* #nosec G402*/
		config.ConnConfig.TLSConfig = &tls.Config{
			RootCAs: caCertPool,
			//nolint:gosec
			InsecureSkipVerify: true,
		}
	}

	dbConnectionPool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return dbConnectionPool, nil
}
