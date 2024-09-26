package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/inventory/transfer"
)

type InventoryClient struct {
	httpUrl   string
	tlsConfig *tls.Config
}

func NewInventoryClient(ctx context.Context, restfulConn *transport.RestfulConfig) (*InventoryClient, error) {
	client := &InventoryClient{}
	err := client.RefreshCredential(ctx, restfulConn)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (c *InventoryClient) RefreshCredential(ctx context.Context, restfulConn *transport.RestfulConfig) error {
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

	return nil
}

func (c *InventoryClient) Request(ctx context.Context, evt cloudevents.Event) error {
	// Extend the other type in the future
	if evt.Type() != string(enum.ManagedClusterInfoType) {
		return nil
	}

	client, err := v1beta1.NewHttpClient(ctx, v1beta1.NewConfig(v1beta1.WithHTTPUrl(c.httpUrl),
		v1beta1.WithHTTPTLSConfig(c.tlsConfig)))
	if err != nil {
		return fmt.Errorf("failed to init the inventory client: %w", err)
	}

	var data []clusterinfov1beta1.ManagedClusterInfo
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	for _, clusterInfo := range data {
		clusterRequest := transfer.GetK8SCluster(&clusterInfo, transfer.GetInventoryClientName(evt.Source()))
		if clusterRequest != nil {
			if _, err := client.K8sClusterService.CreateK8SCluster(ctx, clusterRequest); err != nil {
				return err
			}
		}
	}
	return nil
}
