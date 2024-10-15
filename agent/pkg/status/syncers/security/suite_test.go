package security

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
)

func TestSecurity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Suite")
}

var logger logr.Logger

var _ = BeforeSuite(func() {
	agentConfig := &configs.AgentConfig{
		LeafHubName:          "leafHubName",
		EnableGlobalResource: true,
	}
	configs.SetAgentConfig(agentConfig)
	// Configure logging to write to the Ginkgo writer:
	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
})
