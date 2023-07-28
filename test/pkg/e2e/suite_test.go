package tests

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var (
	rootDir     string
	optionsFile string
	testOptions utils.Options
	testTimeout time.Duration

	testClients utils.TestClient
	httpToken   string
	httpClient  *http.Client

	leafHubNames    []string
	managedClusters []clusterv1.ManagedCluster
)

const (
	ExpectedLeafHubNum        = 2
	ExpectedManagedClusterNum = 2
	Namespace                 = "open-cluster-management"
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
	By("Complete the options and init clients")
	testOptions = completeOptions()
	testClients = utils.NewTestClient(testOptions)

	By("Deploy the global hub")
	deployGlobalHub()

	By("Create the testing rbac")
	err := utils.CreateTestingRBAC(testOptions)
	Expect(err).ShouldNot(HaveOccurred())

	By("Get the http token and client")
	Eventually(func() error {
		httpToken, err = utils.FetchBearerToken(testOptions)
		return err
	}, 1*time.Minute, 1*time.Second*5).ShouldNot(HaveOccurred())
	klog.V(6).Info(fmt.Sprintf("Http BearerToken: %s", httpToken))
	Expect(len(httpToken)).ShouldNot(BeZero())
	httpClient = testClients.HttpClient()

	By("Get the managed clusters")
	Eventually(func() (err error) {
		managedClusters, err = getManagedCluster(httpClient, httpToken)
		return err
	}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	Expect(len(managedClusters)).Should(Equal(ExpectedManagedClusterNum))
})

var _ = AfterSuite(func() {
	utils.DeleteTestingRBAC(testOptions)
})

func completeOptions() utils.Options {
	testTimeout = time.Second * 30

	// get project rootdir path
	exePath, err := os.Executable()
	Expect(err).NotTo(HaveOccurred())

	exeDir := filepath.Dir(exePath)
	rootDir, err = findRootDir(exeDir)
	Expect(err).NotTo(HaveOccurred())

	klog.V(6).Infof("Options Path: %s", optionsFile)
	data, err := os.ReadFile(optionsFile)
	Expect(err).NotTo(HaveOccurred())

	testOptionsContainer := &utils.OptionsContainer{}
	err = yaml.UnmarshalStrict([]byte(data), testOptionsContainer)
	Expect(err).NotTo(HaveOccurred())

	testOptions = testOptionsContainer.Options
	if testOptions.HubCluster.KubeConfig == "" {
		testOptions.HubCluster.KubeConfig = os.Getenv("KUBECONFIG")
	}

	s, _ := json.MarshalIndent(testOptionsContainer, "", "  ")
	klog.V(6).Infof("OptionsContainer %s", s)
	for _, cluster := range testOptions.ManagedClusters {
		if cluster.Name == cluster.LeafHubName {
			leafHubNames = append(leafHubNames, cluster.Name)
		}
	}

	Expect(len(leafHubNames)).Should(Equal(ExpectedLeafHubNum))
	return testOptions
}

func GetClusterID(cluster clusterv1.ManagedCluster) string {
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			return claim.Value
		}
	}
	return ""
}

func deployGlobalHub() {
	By("Creating client for the hub cluster")
	scheme := runtime.NewScheme()
	operatorv1alpha3.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)

	runtimeClient, err := testClients.ControllerRuntimeClient(testOptions.HubCluster.Name, scheme)
	Expect(err).ShouldNot(HaveOccurred())

	By("Deploy postgres and kafka")
	postgresScript := fmt.Sprintf("%s/test/setup/hoh/postgres.sh", rootDir)
	kafkaScript := fmt.Sprintf("%s/test/setup/hoh/kafka.sh", rootDir)
	postgresCmd := exec.Command("bash", postgresScript, rootDir, testOptions.HubCluster.KubeConfig)
	Expect(postgresCmd.Start()).Should(Succeed())
	kafkaCmd := exec.Command("bash", kafkaScript, rootDir, testOptions.HubCluster.KubeConfig)
	Expect(kafkaCmd.Start()).Should(Succeed())

	By("Deploying operator with built image")
	operatorYaml := fmt.Sprintf("%s/operator/config/manager/manager.yaml", rootDir)
	err = replaceContent(operatorYaml, "quay.io/stolostron/multicluster-global-hub-manager:latest",
		testOptions.HubCluster.ManagerImageREF)
	Expect(err).NotTo(HaveOccurred())

	err = replaceContent(operatorYaml, "quay.io/stolostron/multicluster-global-hub-agent:latest",
		testOptions.HubCluster.AgentImageREF)
	Expect(err).NotTo(HaveOccurred())

	cmd := exec.Command("make", "-C", "../../../operator", "deploy")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	setCommandEnv(cmd, "IMG", testOptions.HubCluster.OperatorImageREF, nil)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	Expect(err).ShouldNot(HaveOccurred())

	By("Deploying operand")
	mcgh := &operatorv1alpha3.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multiclusterglobalhub",
			Namespace: "open-cluster-management",
		},
		Spec: operatorv1alpha3.MulticlusterGlobalHubSpec{
			DataLayer: &operatorv1alpha3.DataLayerConfig{
				Type: operatorv1alpha3.LargeScale,
				LargeScale: &operatorv1alpha3.LargeScaleConfig{
					Kafka: &operatorv1alpha3.KafkaConfig{
						TransportFormat: operatorv1alpha3.CloudEvents,
					},
				},
			},
		},
	}
	err = runtimeClient.Create(context.TODO(), mcgh)
	if !errors.IsAlreadyExists(err) {
		Expect(err).ShouldNot(HaveOccurred())
	}

	By("Verifying the multicluster-global-hub postgres/kafka/operator/grafana/manager")
	Eventually(func() error {
		err := postgresCmd.Wait()
		if err != nil {
			return err
		}
		err = kafkaCmd.Wait()
		if err != nil {
			return err
		}

		err = checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-operator")
		if err != nil {
			return err
		}
		err = checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-manager")
		if err != nil {
			return err
		}
		return checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-grafana")
	}, 10*time.Minute, 2*time.Second).Should(Succeed())

	// globalhub setup for e2e
	hack()
}

// e2e setting
func hack() {
	By("Hacking: Update governance-policy-propagator image")
	cmd := exec.Command("kubectl", "patch", "deployment", "governance-policy-propagator", "-n", Namespace, "-p", "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-propagator\",\"image\":\"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0\"}]}}}}")
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	Expect(cmd.Run()).Should(Succeed())

	By("Hacking: Change the placement image")
	cmd = exec.Command("kubectl", "patch", "clustermanager", "cluster-manager", "--type", "merge", "-p", "{\"spec\":{\"placementImagePullSpec\":\"quay.io/open-cluster-management/placement:latest\"}}")
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	Expect(cmd.Run()).Should(Succeed())

	By("Hacking: Expose the manager api by nodeport")
	cmd = exec.Command("kubectl", "apply", "-f", fmt.Sprintf("%s/test/setup/hoh/components/manager-service-local.yaml", rootDir), "-n", Namespace)
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	Expect(cmd.Run()).Should(Succeed())

	By("Hacking: Mutatingwebhook setting")
	cmd = exec.Command("kubectl", "annotate", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "service.beta.openshift.io/inject-cabundle-")
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	output, err := cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred())
	fmt.Println(string(output))

	cmd = exec.Command("kubectl", "get", "secret", "multicluster-global-hub-webhook-certs", "-n", Namespace, "-o", "jsonpath={.data.tls\\.crt}")
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	ca, err := cmd.Output()
	Expect(err).ShouldNot(HaveOccurred())

	cmd = exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "-n", Namespace, "-p", fmt.Sprintf("{\"webhooks\":[{\"name\":\"global-hub.open-cluster-management.io\",\"clientConfig\":{\"caBundle\":\"%s\"}}]}", ca))
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig, os.Environ())
	output, err = cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred())
	fmt.Println(string(output))
}

func setCommandEnv(cmd *exec.Cmd, key, val string, baseEvn []string) {
	if baseEvn != nil {
		cmd.Env = baseEvn
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
}

func replaceContent(file string, old, new string) error {
	cmd := exec.Command("sed", "-i", fmt.Sprintf("s|%s|%s|g", old, new), file)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	return err
}

func checkDeployAvailable(runtimeClient client.Client, namespace, name string) error {
	deployment := &appsv1.Deployment{}
	err := runtimeClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return err
	}
	if deployment.Status.AvailableReplicas > 0 {
		return nil
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		fmt.Printf("deployment image: %s/%s: %s\n", deployment.Name, container.Name, container.Image)
	}
	return fmt.Errorf("deployment: %s is not ready", deployment.Name)
}

// Traverse directories upwards until a directory containing go.mod is found.
func findRootDir(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		if dir == filepath.Dir(dir) {
			return "", fmt.Errorf("rootDir cannot find")
		}

		dir = filepath.Dir(dir)
	}
}
