package config

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func GetRestfulConnBySecret(transportConfig *corev1.Secret) (*transport.RestfulConnCredentail, error) {
	restfulYaml, ok := transportConfig.Data["rest.yaml"]
	if !ok {
		return nil, fmt.Errorf("must set the `rest.yaml` in the transport secret(%s)", transportConfig.Name)
	}
	restfulConn := &transport.RestfulConnCredentail{}
	if err := yaml.Unmarshal(restfulYaml, restfulConn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config to transport credentail: %w", err)
	}

	// decode the ca and client cert
	if restfulConn.CACert != "" {
		bytes, err := base64.StdEncoding.DecodeString(restfulConn.CACert)
		if err != nil {
			return nil, err
		}
		restfulConn.CACert = string(bytes)
	}
	if restfulConn.ClientCert != "" {
		bytes, err := base64.StdEncoding.DecodeString(restfulConn.ClientCert)
		if err != nil {
			return nil, err
		}
		restfulConn.ClientCert = string(bytes)
	}
	if restfulConn.ClientKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(restfulConn.ClientKey)
		if err != nil {
			return nil, err
		}
		restfulConn.ClientKey = string(bytes)
	}
	return restfulConn, nil
}
