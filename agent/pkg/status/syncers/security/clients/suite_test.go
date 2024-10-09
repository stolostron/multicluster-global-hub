package clients

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestClients(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clients Suite")
}

var logger logr.Logger

var _ = BeforeSuite(func() {
	// Configure logging to write to the Ginkgo writer:
	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
})
