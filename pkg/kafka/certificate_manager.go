package kafkaclient

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
)

const (
	ownerOnlyRW      = 0o600
	certFileLocation = "/opt/kafka/ca.crt"
)

var errFailedToAppendCert = errors.New("kafka-certificate-manager: failed to append certificate")

// SetCertificate creates a file with the certificate (PEM) received and registers it in root certification authority.
func SetCertificate(cert *string) (string, error) {
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
