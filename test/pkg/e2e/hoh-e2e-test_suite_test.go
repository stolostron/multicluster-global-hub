package tests

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var (
	optionsFile          string
	testOptions          utils.Options
	testOptionsContainer utils.OptionsContainer
	testTimeout          time.Duration

	clients    utils.Client
	httpToken  string
	httpClient *http.Client

	GlobalHubName             string
	LeafHubNames              []string
	ExpectedLeafHubNum        int
	ExpectedManagedClusterNum int
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

var _ = BeforeSuite(func() {
	initVars()
	deployGlobalHub()

	By("Init the kubernetes client")
	clients = utils.NewTestClient(testOptionsContainer.Options)
	err := utils.CreateTestingRBAC(testOptionsContainer.Options)
	Expect(err).ShouldNot(HaveOccurred())

	By("Init the bearer token")
	Eventually(func() error {
		httpToken, err = utils.FetchBearerToken(testOptions)
		if err != nil {
			return err
		}
		if len(httpToken) > 0 {
			klog.V(6).Info(fmt.Sprintf("Bearer token is ready: %s", httpToken))

			return nil
		} else {
			return fmt.Errorf("token is empty")
		}
	}, 1*time.Minute, 1*time.Second*5).ShouldNot(HaveOccurred())

	By("Init the http client")
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient = &http.Client{Timeout: time.Second * 20, Transport: transport}
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

	GlobalHubName = testOptions.HubCluster.Name

	for _, cluster := range testOptions.ManagedClusters {
		if cluster.Name == cluster.LeafHubName {
			LeafHubNames = append(LeafHubNames, cluster.Name)
		}
	}

	for _, cluster := range testOptions.ManagedClusters {
		if cluster.Name == cluster.LeafHubName {
			ExpectedLeafHubNum += 1
		}
	}

	for _, cluster := range testOptions.ManagedClusters {
		if cluster.Name != cluster.LeafHubName {
			ExpectedManagedClusterNum += 1
		}
	}
}

func GetClusterID(cluster clusterv1.ManagedCluster) string {
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			return claim.Value
		}
	}
	return ""
}
