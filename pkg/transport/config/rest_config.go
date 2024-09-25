package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func GetRestfulConnBySecret(transportSecret *corev1.Secret, c client.Client) (*transport.RestfulConnCredentail, error) {
	restfulYaml, ok := transportSecret.Data["rest.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `rest.yaml` in the transport secret(%s)", transportSecret.Name)
	}
	conn := &transport.RestfulConnCredentail{}
	if err := yaml.Unmarshal(restfulYaml, conn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config to transport credentail: %w", err)
	}

	err := conn.ParseCommonCredentailFromSecret(transportSecret.Namespace, c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the cert credentail: %w", err)
	}
	return conn, nil
}