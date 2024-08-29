// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/x509"
	"encoding/pem"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/klog/v2"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

// default: https://github.com/open-cluster-management-io/addon-framework/blob/main/pkg/utils/csr_helpers.go#L132
func Approve(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn,
	csr *certificatesv1.CertificateSigningRequest,
) bool {
	// if BYO case, then not approve
	if config.IsBYOKafka() {
		return false
	}

	// check org field and commonName field
	block, _ := pem.Decode(csr.Spec.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		klog.Infof("CSR Approve Check Failed csr %q was not recognized: PEM block type is not CERTIFICATE REQUEST", csr.Name)
		return false
	}

	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		klog.Infof("CSR Approve Check Failed csr %q was not recognized: %v", csr.Name, err)
		return false
	}

	// check commonName field
	defaultUser := config.GetKafkaUserName(cluster.Name)
	if defaultUser != x509cr.Subject.CommonName {
		klog.Infof("CSR Approve Check Failed commonName not right; request %s get %s", x509cr.Subject.CommonName, defaultUser)
		return false
	}

	// check userName
	userName := "system:open-cluster-management:" + cluster.Name
	if strings.HasPrefix(csr.Spec.Username, userName) {
		klog.Info("CSR approved")
		return true
	} else {
		klog.Infof("CSR not approved due to illegal requester; want %s get %s", csr.Spec.Username, userName)
		return false
	}
}
