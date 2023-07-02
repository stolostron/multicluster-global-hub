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
	"strings"
	"net/url"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var (
	optionsFile           string
	testOptions           utils.Options
	testOptionsContainer  utils.OptionsContainer
	localOptions          utils.LocalOptions
	localOptionsContainer utils.LocalOptionsContainer
	testTimeout           time.Duration

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
	completeOptions()
	deployGlobalHub()

	// fmt.Println(localOptionsContainer.LocalOptions)
	// fmt.Println("#")
	fmt.Println(localOptions)
	// fmt.Println("##")
	localOptionsContainer.LocalOptions = localOptions
	// fmt.Println(localOptionsContainer.LocalOptions.LocalHubCluster)
	// fmt.Println(localOptionsContainer.LocalOptions.LocalManagedClusters)
	// fmt.Println("###")

	By("Init the kubernetes client")
	clients = utils.NewTestClient(localOptionsContainer.LocalOptions)
	// fmt.Printf("\n clients: \n %v \n", clients)
	err := utils.CreateTestingRBAC(localOptionsContainer.LocalOptions)
	Expect(err).ShouldNot(HaveOccurred())

	By("Init the bearer token")
	Eventually(func() error {
		httpToken, err = utils.FetchBearerToken(localOptions)
		// fmt.Printf("\n Httptoken: \n %v \n", httpToken)
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
	// fmt.Printf("\n transport: \n %v \n", transport)
	httpClient = &http.Client{Timeout: time.Second * 50, Transport: transport}
	// fmt.Printf("\n httpClient: \n %v \n", httpClient)
})

var _ = AfterSuite(func() {
	utils.DeleteTestingRBAC(localOptionsContainer.LocalOptions)
})

func initVars() {
	testTimeout = time.Second * 50
	HUB_CLUSTER_NUM := 2
	MANAGED_CLUSTER_NUM := 1

	klog.V(6).Infof("Options Path: %s", optionsFile)
	data, err := os.ReadFile(optionsFile)
	Expect(err).NotTo(HaveOccurred())

	err = yaml.UnmarshalStrict([]byte(data), &testOptionsContainer)
	Expect(err).NotTo(HaveOccurred())

	testOptions = testOptionsContainer.Options
	localOptions = localOptionsContainer.LocalOptions

	if testOptions.HubCluster.KubeConfig == "" {
		testOptions.HubCluster.KubeConfig = os.Getenv("KUBECONFIG")
	}

	s, _ := json.MarshalIndent(testOptionsContainer, "", "  ")
	klog.V(6).Infof("OptionsContainer %s", s)

	GlobalHubName = "hub-of-hubs"
	localOptions.LocalHubCluster.Name = GlobalHubName
	// localOptionsContainer.LocalOptions.LocalHubCluster.Namespace = "open-cluster-management"
	localOptions.LocalHubCluster.Namespace = "open-cluster-management"
	// localOptionsContainer.LocalOptions.LocalHubCluster.KubeContext = "microshift"
	localOptions.LocalHubCluster.KubeContext = "microshift"

	for i, _ := range testOptions.ManagedClusters {
		LeafHubNames = append(LeafHubNames, fmt.Sprintf("kind-hub%d", i+1))
	}

	ExpectedLeafHubNum = HUB_CLUSTER_NUM
	ExpectedManagedClusterNum = HUB_CLUSTER_NUM * MANAGED_CLUSTER_NUM

	// 把三个kubeconfig复制到localOptions中
	// localOptionsContainer.LocalOptions.LocalHubCluster.KubeConfig = testOptions.HubCluster.KubeConfig
	localOptions.LocalHubCluster.KubeConfig = testOptions.HubCluster.KubeConfig
	for _, cluster := range testOptions.ManagedClusters {
		// kubecontext := strings.TrimPrefix(cluster.KubeConfig, "kubeconfig-")	
		index := strings.LastIndex(cluster.KubeConfig, "kubeconfig")
		kubecontext := cluster.KubeConfig[index+len("kubeconfig"):]

		// localOptionsContainer.LocalOptions.LocalManagedClusters = append(localOptionsContainer.LocalOptions.LocalManagedClusters, utils.LocalManagedCluster{
		// 	KubeConfig: cluster.KubeConfig,
		// 	Name: fmt.Sprintf("kind%s", kubecontext),
		// 	LeafHubName: fmt.Sprintf("kind%s", kubecontext),
		// 	KubeContext: fmt.Sprintf("kind%s", kubecontext),
		// })

		localOptions.LocalManagedClusters = append(localOptions.LocalManagedClusters, utils.LocalManagedCluster{
			KubeConfig: cluster.KubeConfig,
			Name: fmt.Sprintf("kind%s", kubecontext),
			LeafHubName: fmt.Sprintf("kind%s", kubecontext),
			KubeContext: fmt.Sprintf("kind%s", kubecontext),
		})
	}
}

func completeOptions() {
	currentDir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	rootDir := fmt.Sprintf("%s/../../..", currentDir)
	
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = testOptions.HubCluster.KubeConfig
	configOverrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := clientConfig.RawConfig()
	if err != nil {
		fmt.Println("Failed to load kubeconfig:", err)
		os.Exit(1)
	}
	config.CurrentContext = "microshift"
	cluster := config.Clusters[config.Contexts[config.CurrentContext].Cluster]
	hub_api_server := cluster.Server

	localOptions.LocalHubCluster.ApiServer = hub_api_server

	parsedURL, err := url.Parse(hub_api_server)
	if err != nil {
		fmt.Println("无法解析URL:", err)
		return
	}
	parsedURL.Host = fmt.Sprintf("%s:%s", parsedURL.Hostname(), "30080")
	newURL := parsedURL.String()
	localOptions.LocalHubCluster.Nonk8sApiServer = newURL

	localOptions.LocalHubCluster.ManagerImageREF = "quay.io/stolostron/multicluster-global-hub-manager:latest"
	localOptions.LocalHubCluster.AgentImageREF = "quay.io/stolostron/multicluster-global-hub-agent:latest"
	localOptions.LocalHubCluster.OperatorImageREF = "quay.io/stolostron/multicluster-global-hub-operator:latest"

	localOptions.LocalHubCluster.CrdsDir = rootDir+"/pkg/testdata/crds"
	if (os.Getenv("IS_CANARY_ENV") == "true") {
		localOptions.LocalHubCluster.StoragePath = rootDir+"/operator/config/samples/storage/deploy_postgres.sh"
		localOptions.LocalHubCluster.TransportPath = rootDir+"/operator/config/samples/transport/deploy_kafka.sh"
	} else {
		localOptions.LocalHubCluster.StoragePath = rootDir+"/test/setup/hoh/postgres_setup.sh"
		localOptions.LocalHubCluster.TransportPath = rootDir+"/test/setup/hoh/kafka_setup.sh"
	}

	cmd := exec.Command("docker", "inspect", "-f", "'{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'", "hub-of-hubs")
	container_node_ip, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(err)
	}

	localOptions.LocalHubCluster.DatabaseExternalHost = strings.Trim(string(container_node_ip), "'\n")
	localOptions.LocalHubCluster.DatabaseExternalPort = 32432
	fmt.Printf("\n localOptions.LocalHubCluster.DatabaseExternalHost: \n %v\n", localOptions.LocalHubCluster.DatabaseExternalHost)
}

func GetClusterID(cluster clusterv1.ManagedCluster) string {
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			return claim.Value
		}
	}
	return ""
}
