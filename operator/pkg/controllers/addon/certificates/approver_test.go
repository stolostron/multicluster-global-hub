package certificates

import (
	"context"
	"crypto/x509/pkix"
	"testing"

	"github.com/stretchr/testify/assert"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

func TestCSRApprover(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects().Build()
	err := config.SetBYOKafka(context.Background(), fakeClient, "default")
	assert.Nil(t, err)
	assert.Equal(t, config.IsBYOKafka(), false)

	cases := []struct {
		name     string
		csr      *certificatesv1.CertificateSigningRequest
		cluster  *clusterv1.ManagedCluster
		approved bool
	}{
		{
			name:     "approve csr",
			csr:      newCSR(config.GetKafkaUserName("cluster1"), "cluster1"),
			cluster:  newCluster("cluster1"),
			approved: true,
		},
		{
			name:     "requester is not correct",
			csr:      newCSR(config.GetKafkaUserName("cluster1"), "cluster2"),
			cluster:  newCluster("cluster1"),
			approved: false,
		},
		{
			name:     "common name is not correct",
			csr:      newCSR("test", "cluster1"),
			cluster:  newCluster("cluster1"),
			approved: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			approved := Approve(c.cluster, nil, c.csr)
			assert.Equal(t, c.approved, approved)
		})
	}
}

func newCSR(commonName string, clusterName string, orgs ...string) *certificatesv1.CertificateSigningRequest {
	clientKey, _ := keyutil.MakeEllipticPrivateKeyPEM()
	privateKey, _ := keyutil.ParsePrivateKeyPEM(clientKey)

	// request, _ := certutil.MakeCSR(privateKey, &pkix.Name{CommonName: commonName, Organization: orgs}, []string{"test.localhost"}, nil)

	// Generate a x509 certificate signing request.
	csrPEM, _ := certutil.MakeCSR(privateKey, &pkix.Name{CommonName: commonName, Organization: orgs}, nil, nil)

	return &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageClientAuth,
			},
			Username: "system:open-cluster-management:" + clusterName,
			Request:  csrPEM,
		},
	}
}

func newCluster(name string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
