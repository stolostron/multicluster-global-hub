package config

import (
	"io/ioutil"
	"strings"
	"testing"

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
				BootstrapServer: "localhost:9092",
				EnableTLS:       true,
				CaCertPath:      "/tmp/ca.crt",
				ClientCertPath:  "/tmp/client.crt",
				ClientKeyPath:   "/tmp/client.key",
			},
			expectedErr: nil,
		},
		{
			desc: "kafka config without tls",
			kafkaConfig: &transport.KafkaConfig{
				BootstrapServer: "localhost:9092",
				EnableTLS:       false,
			},
			expectedErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := GetConfluentConfigMap(tc.kafkaConfig)
			if err != tc.expectedErr {
				t.Errorf("%s:\nexpected err: %v\ngot err: %v\n", tc.desc, tc.expectedErr, err)
			}
		})
	}
}

func TestGetSaramaConfig(t *testing.T) {
	kafkaConfig := &transport.KafkaConfig{
		EnableTLS:      false,
		ClientCertPath: "/tmp/client.crt",
		ClientKeyPath:  "/tmp/client.key",
		CaCertPath:     "/tmp/ca.crt",
	}
	_, err := GetSaramaConfig(kafkaConfig)
	if err != nil {
		t.Errorf("failed to get sarama config - %v", err)
	}

	kafkaConfig.EnableTLS = true
	if er := ioutil.WriteFile(kafkaConfig.CaCertPath, []byte("test"), 0o644); er != nil { // #nosec G304
		t.Errorf("failed to write cert file - %v", er)
	}
	_, err = GetSaramaConfig(kafkaConfig)
	if err != nil && !strings.Contains(err.Error(), "failed to find any PEM data in certificate") {
		t.Errorf("failed to get sarama config - %v", err)
	}
}
