package transport_test

import (
	"fmt"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
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

	By("Create mock kafka cluster")
	var err error
	mockCluster, err = kafka.NewMockCluster(1)
	Expect(err).NotTo(HaveOccurred())
	if _, err := fmt.Fprintf(GinkgoWriter, "mock kafka bootstrap server address: %s\n", mockCluster.BootstrapServers()); err != nil {
		panic(err)
	}
})

var _ = AfterSuite(func() {
	mockCluster.Close()
})
