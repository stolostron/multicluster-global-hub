package helpers

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

const (
	ownerOnlyRW      = 0o600
	certFileLocation = "/opt/kafka/ca.crt"
)

var errFailedToAppendCert = errors.New("kafka-certificate-manager: failed to append certificate")

func LoadSslToConfigMap(sslCA string, kafkaConfigMap *kafka.ConfigMap) error {
	// sslBase64EncodedCertificate
	if sslCA != "" {
		certFileLocation, err := setCertificate(&sslCA)
		if err != nil {
			return fmt.Errorf("failed to SetCertificate - %w", err)
		}

		if err = kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return fmt.Errorf("failed to SetKey security.protocol - %w", err)
		}

		if err = kafkaConfigMap.SetKey("ssl.ca.location", certFileLocation); err != nil {
			return fmt.Errorf("failed to SetKey ssl.ca.location - %w", err)
		}
	}
	return nil
}

// SetCertificate creates a file with the certificate (PEM) received and registers it in root certification authority.
func setCertificate(cert *string) (string, error) {
	certBytes, err := base64.StdEncoding.DecodeString(*cert)
	if err != nil {
		return "", fmt.Errorf("kafka-certificate-manager failed to decode certificate - %w", err)
	}

	if err = ioutil.WriteFile(certFileLocation, certBytes, ownerOnlyRW); err != nil {
		return "", fmt.Errorf("kafka-certificate-manager failed to write certificate - %w", err)
	}

	// Get the SystemCertPool, continue with an empty pool on error
	rootCertAuth, _ := x509.SystemCertPool()
	if rootCertAuth == nil {
		rootCertAuth = x509.NewCertPool()
	}

	// Append our cert to the system pool
	if ok := rootCertAuth.AppendCertsFromPEM(certBytes); !ok {
		return "", errFailedToAppendCert
	}

	return certFileLocation, nil
}

func ToByteArray(i int) []byte {
	arr := make([]byte, 4)
	binary.BigEndian.PutUint32(arr[0:4], uint32(i))

	return arr
}
