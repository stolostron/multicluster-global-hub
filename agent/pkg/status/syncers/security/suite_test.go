package security

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	zaplogger "github.com/stolostron/multicluster-global-hub/pkg/logger"
)

func TestSecurity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Suite")
}

var logger *zap.SugaredLogger

var _ = BeforeSuite(func() {
	agentConfig := &configs.AgentConfig{
		LeafHubName: "leafHubName",
	}
	configs.SetAgentConfig(agentConfig)
	// Configure logging to write to the Ginkgo writer:
	logger = zaplogger.DefaultZapLogger()
})
