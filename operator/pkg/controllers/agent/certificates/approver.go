// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/x509"
	"encoding/pem"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// default: https://github.com/open-cluster-management-io/addon-framework/blob/main/pkg/utils/csr_helpers.go#L132
func Approve(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn,
	csr *certificatesv1.CertificateSigningRequest,
) bool {
	// only if use the built-in kafka or inventory, then approve the csr
	if !config.IsBYOKafka() && !config.EnableInventory() {
		return false
	}

	// check org field and commonName field
	block, _ := pem.Decode(csr.Spec.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		log.Infof("CSR Approve Check Failed csr %q was not recognized: PEM block type is not CERTIFICATE REQUEST", csr.Name)
		return false
	}

	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		log.Infof("CSR Approve Check Failed csr %q was not recognized: %v", csr.Name, err)
		return false
	}

	// check commonName field
	defaultUser := config.GetTransportConfigClientName(cluster.Name)
	if defaultUser != x509cr.Subject.CommonName {
		log.Infof("CSR Approve Check Failed CN not right; request %s get %s", x509cr.Subject.CommonName, defaultUser)
		return false
	}

	// check userName
	userName := "system:open-cluster-management:" + cluster.Name
	if strings.HasPrefix(csr.Spec.Username, userName) {
		log.Infof("CSR approved for user: %s", defaultUser)
		return true
	} else {
		log.Infof("CSR not approved due to illegal requester; want %s get %s", csr.Spec.Username, userName)
		return false
	}
}
