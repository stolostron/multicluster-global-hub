package config

import (
	"log"
	"os"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func GetConfluentConfigMap() (*kafka.ConfigMap, error) {
	secret, err := GetTransportSecret()
	if err != nil {
		log.Fatalf("failed to get transport secret: %v", err)
		return nil, err
	}
	bootStrapServer := secret.Data["bootstrap_server"]

	caCrtPath := "/tmp/ca.crt"
	err = os.WriteFile(caCrtPath, secret.Data["ca.crt"], 0o644)
	if err != nil {
		log.Fatalf("failed to write ca.crt: %v", err)
		return nil, err
	}

	clientCrtPath := "/tmp/client.crt"
	err = os.WriteFile(clientCrtPath, secret.Data["client.crt"], 0o644)
	if err != nil {
		log.Fatalf("failed to write client.crt: %v", err)
		return nil, err
	}

	clientKeyPath := "/tmp/client.key"
	err = os.WriteFile(clientKeyPath, secret.Data["client.key"], 0o644)
	if err != nil {
		log.Fatalf("failed to write client.key: %v", err)
		return nil, err
	}

	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":        string(bootStrapServer),
		"socket.keepalive.enable":  "false",
		"log.connection.close":     "false", // silence spontaneous disconnection logs, kafka recovers by itself.
		"security.protocol":        "ssl",
		"ssl.ca.location":          caCrtPath,
		"ssl.certificate.location": clientCrtPath,
		"ssl.key.location":         clientKeyPath,
	}
	return kafkaConfigMap, nil
}
