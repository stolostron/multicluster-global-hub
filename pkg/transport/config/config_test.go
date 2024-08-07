package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestConfluentConfig(t *testing.T) {
	cases := []struct {
		desc        string
		kafkaConfig *transport.KafkaConfig
		expectedErr error
	}{
		{
			desc: "kafka config with tls",
			kafkaConfig: &transport.KafkaConfig{
				ConnCredential: &transport.ConnCredential{
					BootstrapServer: "localhost:9092",
					CACert:          "",
					ClientCert:      "clientca",
					ClientKey:       "clientkey",
				},
				EnableTLS: true,
			},
			expectedErr: nil,
		},
		{
			desc: "kafka config without tls",
			kafkaConfig: &transport.KafkaConfig{
				ConnCredential: &transport.ConnCredential{
					BootstrapServer: "localhost:9092",
					CACert:          "",
					ClientCert:      "clientca",
					ClientKey:       "clientkey",
				},
				EnableTLS: false,
			},
			expectedErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := GetConfluentConfigMap(tc.kafkaConfig, true)
			if tc.expectedErr != nil {
				assert.Equal(t, err.Error(), tc.expectedErr.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestGetSaramaConfig(t *testing.T) {
	kafkaConfig := &transport.KafkaConfig{
		ConnCredential: &transport.ConnCredential{
			BootstrapServer: "localhost:9092",
			CACert:          "",
			ClientCert:      "clientca",
			ClientKey:       "clientkey",
		},
		EnableTLS: false,
	}
	_, err := GetSaramaConfig(kafkaConfig)
	if err != nil {
		t.Errorf("failed to get sarama config - %v", err)
	}

	kafkaConfig.EnableTLS = true
	_, err = GetSaramaConfig(kafkaConfig)
	if err != nil && !strings.Contains(err.Error(), "failed to find any PEM data in certificate") {
		t.Errorf("failed to get sarama config - %v", err)
	}
}
