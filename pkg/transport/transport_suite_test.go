package transport_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var mockCluster *kafka.MockCluster

func TestTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Transport Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	By("Create mock kafka cluster")
	var err error
	mockCluster, err = kafka.NewMockCluster(1)
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "mock kafka bootstrap server address: %s\n", mockCluster.BootstrapServers())
})

var _ = AfterSuite(func() {
	mockCluster.Close()
})
