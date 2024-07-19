// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/klog/v2"
)

// default: https://github.com/open-cluster-management-io/addon-framework/blob/main/pkg/utils/csr_helpers.go#L65
func Sign(csr *certificatesv1.CertificateSigningRequest, clientCaKey, clientCaCert []byte) []byte {
	caKey, caCert, err := parseClientCA(clientCaKey, clientCaCert)
	if err != nil {
		klog.Infof("The singer checks CSR(%s), not get client CA: %v", csr.Name, err)
		return nil
	}

	var usages []string
	for _, usage := range csr.Spec.Usages {
		usages = append(usages, string(usage))
	}

	certExpiryDuration := 365 * 24 * time.Hour
	durationUntilExpiry := time.Until(caCert.NotAfter)
	if durationUntilExpiry <= 0 {
		klog.Infof("The signer has expired, expired time: %v", caCert.NotAfter)
		return nil
	}
	if durationUntilExpiry < certExpiryDuration {
		certExpiryDuration = durationUntilExpiry
	}

	policy := &config.Signing{
		Default: &config.SigningProfile{
			Usage:        usages,
			Expiry:       certExpiryDuration,
			ExpiryString: certExpiryDuration.String(),
		},
	}
	singer, err := local.NewSigner(caKey, caCert, signer.DefaultSigAlgo(caKey), policy)
	if err != nil {
		klog.Infof("failed to create new local signer: %v", err)
		return nil
	}

	signedCert, err := singer.Sign(signer.SignRequest{
		Request: string(csr.Spec.Request),
	})
	if err != nil {
		klog.Infof("failed to sign the CSR(%s): %v", csr.Name, err)
		return nil
	}
	return signedCert
}

func parseClientCA(caKey, caCert []byte) (crypto.Signer, *x509.Certificate, error) {
	if caKey == nil || caCert == nil {
		return nil, nil, fmt.Errorf("the client CA is not ready")
	}
	block1, _ := pem.Decode(caCert)
	if block1 == nil {
		return nil, nil, fmt.Errorf("cannot decode ca.crt: %v", caCert)
	}
	caCerts, err := x509.ParseCertificates(block1.Bytes)
	if err != nil {
		klog.Errorf("failed to parse the cert: %v", err)
		return nil, nil, err
	}

	signer, err := DecodePrivateKeyBytes(caKey)
	if err != nil {
		klog.Errorf("failed to decode private key: %v", err)
		return nil, nil, err
	}
	return signer, caCerts[0], nil
}

// DecodePrivateKeyBytes will decode a PEM encoded private key into a crypto.Signer.
// It supports ECDSA and RSA private keys only. All other types will return err.
func DecodePrivateKeyBytes(keyBytes []byte) (crypto.Signer, error) {
	// decode the private key pem
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("error decoding private key PEM block")
	}

	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing pkcs#8 private key: %s", err.Error())
		}

		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("error parsing pkcs#8 private key: invalid key type")
		}
		return signer, nil
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing ecdsa private key: %s", err.Error())
		}

		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing rsa private key: %s", err.Error())
		}
		err = key.Validate()
		if err != nil {
			return nil, fmt.Errorf("rsa private key failed validation: %s", err.Error())
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unknown private key type: %s", block.Type)
	}
}
