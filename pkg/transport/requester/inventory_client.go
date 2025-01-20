package requester

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/project-kessel/inventory-client-go/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type InventoryClient struct {
	httpUrl   string
	tlsConfig *tls.Config

	httpClient *v1beta1.InventoryHttpClient
}

func NewInventoryClient(ctx context.Context, restfulConn *transport.RestfulConfig) (*InventoryClient, error) {
	c := &InventoryClient{}
	err := c.RefreshClient(ctx, restfulConn)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *InventoryClient) RefreshClient(ctx context.Context, restfulConn *transport.RestfulConfig) error {
	clientCert, err := tls.X509KeyPair([]byte(restfulConn.ClientCert), []byte(restfulConn.ClientKey))
	if err != nil {
		return fmt.Errorf("failed the load client cert from raw data: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(restfulConn.CACert)) {
		return fmt.Errorf("failed to append CA certificate to pool")
	}

	// #nosec G402
	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
	}

	c.httpUrl = restfulConn.Host
	c.tlsConfig = &tlsConfig
	c.httpClient, err = v1beta1.NewHttpClient(ctx, v1beta1.NewConfig(v1beta1.WithHTTPUrl(c.httpUrl),
		v1beta1.WithHTTPTLSConfig(c.tlsConfig)))
	if err != nil {
		return fmt.Errorf("failed to init the inventory http client: %w", err)
	}
	return nil
}

func (c *InventoryClient) GetHttpClient() *v1beta1.InventoryHttpClient {
	return c.httpClient
}
