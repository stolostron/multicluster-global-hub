package tests

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"
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
})

var _ = AfterSuite(func() {
	utils.DeleteTestingRBAC(testOptionsContainer.Options)
})

func initVars() {
	testTimeout = time.Second * 30

	klog.V(6).Infof("Options Path: %s", optionsFile)
	data, err := ioutil.ReadFile(optionsFile)
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
