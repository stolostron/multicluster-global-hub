package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestConfluentConfig(t *testing.T) {
	cases := []struct {
		desc        string
		kafkaConfig *transport.KafkaInternalConfig
		expectedErr error
	}{
		{
			desc: "kafka config with tls",
			kafkaConfig: &transport.KafkaInternalConfig{
				BootstrapServer: "localhost:9092",
				EnableTLS:       true,
				CaCertPath:      "/tmp/ca.crt",
				ClientCertPath:  "/tmp/client.crt",
				ClientKeyPath:   "/tmp/client.key",
			},
			expectedErr: errors.New("failed to append ca certificate"),
		},
		{
			desc: "kafka config without tls",
			kafkaConfig: &transport.KafkaInternalConfig{
				BootstrapServer: "localhost:9092",
				EnableTLS:       false,
			},
			expectedErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.kafkaConfig.CaCertPath != "" {
				assert.Nil(t, os.WriteFile(tc.kafkaConfig.CaCertPath, []byte("cadata"), 0o644))
			}
			if tc.kafkaConfig.ClientCertPath != "" {
				assert.Nil(t, os.WriteFile(tc.kafkaConfig.ClientCertPath, []byte("certdata"), 0o644))
			}
			if tc.kafkaConfig.ClientKeyPath != "" {
				assert.Nil(t, os.WriteFile(tc.kafkaConfig.ClientKeyPath, []byte("keydata"), 0o644))
			}
			_, err := GetConfluentConfigMap(tc.kafkaConfig, true)
			if tc.expectedErr != nil {
				assert.Equal(t, err.Error(), tc.expectedErr.Error())
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestSetConsumerConfig(t *testing.T) {
	kafkaConfigMap := GetBasicConfigMap()
	SetConsumerConfig(kafkaConfigMap, "test-group", 100)
	val, err := kafkaConfigMap.Get("group.id", nil)
	assert.Nil(t, err)
	assert.Equal(t, "test-group", val)

	val, err = kafkaConfigMap.Get("auto.offset.reset", nil)
	assert.Nil(t, err)
	assert.Equal(t, "earliest", val)

	val, err = kafkaConfigMap.Get("enable.auto.commit", nil)
	assert.Nil(t, err)
	assert.Equal(t, "true", val)

	val, err = kafkaConfigMap.Get("max.partition.fetch.bytes", nil)
	assert.Nil(t, err)
	assert.Equal(t, MaxSizeToFetch, val)

	val, err = kafkaConfigMap.Get("fetch.message.max.bytes", nil)
	assert.Nil(t, err)
	assert.Equal(t, MaxSizeToFetch, val)

	val, err = kafkaConfigMap.Get("metadata.max.age.ms", nil)
	assert.Nil(t, err)
	assert.Equal(t, "100", val)

	val, err = kafkaConfigMap.Get("topic.metadata.refresh.interval.ms", nil)
	assert.Nil(t, err)
	assert.Equal(t, "100", val)
}

func TestGetSaramaConfig(t *testing.T) {
	kafkaConfig := &transport.KafkaInternalConfig{
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
	if er := os.WriteFile(kafkaConfig.CaCertPath, []byte("test"), 0o644); er != nil { // #nosec G304
		t.Errorf("failed to write cert file - %v", er)
	}
	_, err = GetSaramaConfig(kafkaConfig)
	if err != nil && !strings.Contains(err.Error(), "failed to find any PEM data in certificate") {
		t.Errorf("failed to get sarama config - %v", err)
	}
}
