package config

import (
	"crypto/x509"
	"fmt"
	"os"

	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func LoadCACertToConfigMap(kafkaCAPath string, kafkaConfigMap *kafkav2.ConfigMap) error {
	// If kafka CA certificate path is not empty, set ssl for kafka client configuration
	if kafkaCAPath != "" {
		// Read in the cert file
		kafkaCACert, err := os.ReadFile(kafkaCAPath) // #nosec G304
		if err != nil {
			return fmt.Errorf("failed to read ca certificate from %s - %w", kafkaCAPath, err)
		}

		if err = appendCACertificate(kafkaCACert); err != nil {
			return fmt.Errorf("failed to append kafka ca certificate - %w", err)
		}

		if err = kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return fmt.Errorf("failed to SetKey security.protocol - %w", err)
		}

		if err = kafkaConfigMap.SetKey("ssl.ca.location", kafkaCAPath); err != nil {
			return fmt.Errorf("failed to SetKey ssl.ca.location - %w", err)
		}
	}

	return nil
}

// appendCACertificate registers the ca certificate (PEM) in root certification authority.
func appendCACertificate(caCertBytes []byte) error {
	// Get the SystemCertPool, continue with an empty pool on error
	rootCertAuth, _ := x509.SystemCertPool()
	if rootCertAuth == nil {
		rootCertAuth = x509.NewCertPool()
	}

	// Append the ca certificate to the system pool
	if ok := rootCertAuth.AppendCertsFromPEM(caCertBytes); !ok {
		return fmt.Errorf("failed to append ca certificate to system ca certificate pool")
	}

	return nil
}
