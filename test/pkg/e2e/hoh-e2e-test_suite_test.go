package tests

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var (
	optionsFile          string
	testOptions          utils.Options
	testOptionsContainer utils.OptionsContainer
	testTimeout          time.Duration
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)
	flag.StringVar(&optionsFile, "options", "", "Location of an \"options.yaml\" file to provide input for various tests")
}

var clients utils.Client

var _ = BeforeSuite(func() {
	initVars()
	clients = utils.NewTestClient(testOptionsContainer.Options)
	err := utils.CreateTestingRBAC(testOptionsContainer.Options)
	Expect(err).ShouldNot(HaveOccurred())
	// Check the bearer token is ready
	Eventually(func() error {
		token, err := utils.FetchBearerToken(testOptions)
		if err != nil {
			return err
		}
		if len(token) > 0 {
			klog.V(6).Info(fmt.Sprintf("Bearer token is ready: %s", token))
			return nil
		} else {
			return fmt.Errorf("token is empty")
		}
	}, 1*time.Minute, 1*time.Second*5).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	utils.DeleteTestingRBAC(testOptionsContainer.Options)
})

func initVars() {
	testTimeout = time.Second * 30

	klog.V(6).Infof("Options Path: %s", optionsFile)
	data, err := os.ReadFile(optionsFile)
	Expect(err).NotTo(HaveOccurred())

	err = yaml.UnmarshalStrict([]byte(data), &testOptionsContainer)
	Expect(err).NotTo(HaveOccurred())

	testOptions = testOptionsContainer.Options

	if testOptions.HubCluster.KubeConfig == "" {
		testOptions.HubCluster.KubeConfig = os.Getenv("KUBECONFIG")
	}

	s, _ := json.MarshalIndent(testOptionsContainer, "", "  ")
	klog.V(6).Infof("OptionsContainer %s", s)
}
