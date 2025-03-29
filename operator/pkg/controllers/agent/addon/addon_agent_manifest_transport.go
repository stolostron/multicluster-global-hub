package addon

import (
	"context"
	"encoding/base64"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func setTransportConfigs(manifestsConfig *config.ManifestsConfig,
	cluster *clusterv1.ManagedCluster, c client.Client,
) error {
	if config.EnableInventory() {
		inventoryConn, err := getInventoryCredential(c)
		if err != nil {
			return err
		}
		inventoryConfigYaml, err := inventoryConn.YamlMarshal(false)
		if err != nil {
			return fmt.Errorf("failed to marshalling the inventory config yaml: %w", err)
		}
		manifestsConfig.InventoryConfigYaml = base64.StdEncoding.EncodeToString(inventoryConfigYaml)
		manifestsConfig.InventoryServerCASecret = inventoryConn.CASecretName
		manifestsConfig.InventoryServerCACert = inventoryConn.CACert
		return nil
	}

	// kafka setup
	transporter := config.GetTransporter()

	_, err := transporter.EnsureTopic(cluster.Name)
	if err != nil {
		return err
	}
	// this controller might be triggered by global hub controller(like the topics changes), so we also need to
	// update the authz for the topic
	_, err = transporter.EnsureUser(cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to update the kafkauser for the cluster(%s): %v", cluster.Name, err)
	}

	// will block until the credential is ready
	kafkaConnection, err := transporter.GetConnCredential(cluster.Name)
	if err != nil {
		return err
	}
	kafkaConfigYaml, err := kafkaConnection.YamlMarshal(config.IsBYOKafka())
	if err != nil {
		return fmt.Errorf("failed to marshalling the kafka config yaml: %w", err)
	}
	manifestsConfig.KafkaConfigYaml = base64.StdEncoding.EncodeToString(kafkaConfigYaml)
	// render the cluster ca whether under the BYO cases
	manifestsConfig.KafkaClusterCASecret = kafkaConnection.CASecretName
	manifestsConfig.KafkaClusterCACert = kafkaConnection.CACert
	return nil
}

func getInventoryCredential(c client.Client) (*transport.RestfulConfig, error) {
	inventoryCredential := &transport.RestfulConfig{}

	// add ca
	serverCASecretName := certificates.InventoryServerCASecretName
	inventoryNamespace := config.GetMGHNamespacedName().Namespace
	_, serverCACert, err := certificates.GetKeyAndCert(c, inventoryNamespace, serverCASecretName)
	if err != nil {
		return nil, fmt.Errorf("failed to get the inventory server ca: %w", err)
	}
	inventoryCredential.CASecretName = serverCASecretName
	inventoryCredential.CACert = base64.StdEncoding.EncodeToString(serverCACert)

	// add client
	inventoryCredential.ClientSecretName = config.AgentCertificateSecretName()

	inventoryRoute := &routev1.Route{}
	err = c.Get(context.Background(), types.NamespacedName{
		Name:      constants.InventoryRouteName,
		Namespace: inventoryNamespace,
	}, inventoryRoute)
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory route: %s/%s", inventoryNamespace, constants.InventoryRouteName)
	}

	// host
	inventoryCredential.Host = fmt.Sprintf("https://%s:443", inventoryRoute.Spec.Host)

	return inventoryCredential, nil
}
