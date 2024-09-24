package config

import (
	"context"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// decode the ca and client cert
	if conn.CACert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.CACert)
		if err != nil {
			return nil, err
		}
		conn.CACert = string(bytes)
	}
	if conn.ClientCert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientCert)
		if err != nil {
			return nil, err
		}
		conn.ClientCert = string(bytes)
	}
	if conn.ClientKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientKey)
		if err != nil {
			return nil, err
		}
		conn.ClientKey = string(bytes)
	}

	if conn.CASecretName != "" {
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: transportSecret.Namespace,
				Name:      conn.CASecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(caSecret), caSecret); err != nil {
			return nil, err
		}
		conn.CACert = string(caSecret.Data["ca.crt"])
	}
	if conn.ClientSecretName != "" {
		clientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: transportSecret.Namespace,
				Name:      conn.ClientSecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(clientSecret), clientSecret); err != nil {
			return nil, fmt.Errorf("failed to get the client cert: %w", err)
		}
		conn.ClientCert = string(clientSecret.Data["tls.crt"])
		conn.ClientKey = string(clientSecret.Data["tls.key"])
		if conn.ClientCert == "" || conn.ClientKey == "" {
			return nil, fmt.Errorf("the client cert or key must not be empty: %s", conn.ClientSecretName)
		}
	}
	return conn, nil
}
