package config

import (
	"github.com/stolostron/multicluster-global-hub/operator/pkg/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// MiddlewareConfig defines the configuration for middleware and shared in opearator
type MiddlewareConfig struct {
	StorageConn   *postgres.PostgresConnection
	TransportConn *transport.ConnCredential
}

var middlewareCfg = &MiddlewareConfig{}

func SetStorageConn(conn *postgres.PostgresConnection) {
	middlewareCfg.StorageConn = conn
}

func SetTransportConn(conn *transport.ConnCredential) {
	middlewareCfg.TransportConn = conn
}

func GetMiddlewareConfig() *MiddlewareConfig {
	return middlewareCfg
}
