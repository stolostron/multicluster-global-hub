// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package nonk8sapi_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	ctx                     context.Context
	cancel                  context.CancelFunc
	testPostgres            *testpostgres.TestPostgres
	testAuthServer          *httptest.Server
	testAuthServerCAPEMPath string
)

func TestNonK8sAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NonK8s API Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	var err error

	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
	err = testpostgres.InitDatabase(testPostgres.URI)
	Expect(err).NotTo(HaveOccurred())

	testAuthServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"kind": "User",
			"apiVersion": "user.openshift.io/v1",
			"metadata": {
			  "name": "kube:admin",
			  "creationTimestamp": null
			},
			"groups": [
			  "system:authenticated",
			  "system:cluster-admins"
			]
		  }`))
	}))

	certDER := testAuthServer.TLS.Certificates[0].Certificate[0]
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	caFile, err := os.CreateTemp("", "cluster-api-ca-*.pem")
	Expect(err).NotTo(HaveOccurred(), "cluster API CA temp file must be created for auth middleware tests")
	DeferCleanup(func() {
		Expect(os.Remove(caFile.Name())).To(Succeed())
	})
	_, err = caFile.Write(certPEM)
	Expect(err).NotTo(HaveOccurred(), "cluster API CA certificate PEM must be written to temp file")
	err = caFile.Close()
	Expect(err).NotTo(HaveOccurred(), "cluster API CA temp file must be closed after write")
	testAuthServerCAPEMPath = caFile.Name()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	testAuthServer.Close()
	err := testPostgres.Stop()
	Expect(err).NotTo(HaveOccurred())
	cancel()
})
