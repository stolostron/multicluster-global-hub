package clients

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	zaplogger "github.com/stolostron/multicluster-global-hub/pkg/logger"
	"go.uber.org/zap"
)

func TestClients(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clients Suite")
}

var logger *zap.SugaredLogger

var _ = BeforeSuite(func() {
	// Configure logging to write to the Ginkgo writer:
	logger = zaplogger.ZapLogger("client-test")
})
