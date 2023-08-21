package tests

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kustomize"
	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var (
	rootDir     string
	optionsFile string
	testOptions utils.Options
	testTimeout time.Duration

	testClients utils.TestClient
	httpClient  *http.Client

	leafHubNames    []string
	managedClusters []clusterv1.ManagedCluster
	mcgh            *globalhubv1alpha4.MulticlusterGlobalHub
	scheme          *runtime.Scheme
)

const (
	ExpectedLeafHubNum        = 2
	ExpectedManagedClusterNum = 2
	Namespace                 = "open-cluster-management"
	ServiceMonitorNamespace   = "openshift-monitoring"
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
	httpClient = testClients.HttpClient()

	By("Deploy the global hub")
	deployGlobalHub()

	By("Get the managed clusters")
	Eventually(func() (err error) {
		managedClusters, err = getManagedCluster(httpClient)
		return err
	}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	Expect(len(managedClusters)).Should(Equal(ExpectedManagedClusterNum))
})

var _ = AfterSuite(func() {
	// utils.DeleteTestingRBAC(testOptions)
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
	if testOptions.GlobalHub.KubeConfig == "" {
		testOptions.GlobalHub.KubeConfig = os.Getenv("KUBECONFIG")
	}

	s, _ := json.MarshalIndent(testOptionsContainer, "", "  ")
	klog.V(6).Infof("OptionsContainer %s", s)
	for _, cluster := range testOptions.GlobalHub.ManagedHubs {
		leafHubNames = append(leafHubNames, cluster.Name)
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

func deployGlobalHub() {
	By("Creating client for the hub cluster")
	scheme = runtime.NewScheme()
	Expect(globalhubv1alpha4.AddToScheme(scheme)).Should(Succeed())
	Expect(appsv1.AddToScheme(scheme)).Should(Succeed())

	By("Creating namespace for the ServiceMonitor")
	_, err := testClients.KubeClient().CoreV1().Namespaces().Get(context.Background(), ServiceMonitorNamespace,
		metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		_, err = testClients.KubeClient().CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ServiceMonitorNamespace,
			},
		}, metav1.CreateOptions{})
	}
	Expect(err).NotTo(HaveOccurred())

	Expect(kustomize.Apply(testClients, testOptions,
		kustomize.Options{KustomizationPath: fmt.Sprintf("%s/test/pkg/e2e/resources", rootDir)})).NotTo(HaveOccurred())
	Expect(kustomize.Apply(testClients, testOptions,
		kustomize.Options{KustomizationPath: fmt.Sprintf("%s/operator/config/default", rootDir)})).NotTo(HaveOccurred())

	By("Deploying operand")
	mcgh = &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multiclusterglobalhub",
			Namespace: "open-cluster-management",
			Annotations: map[string]string{
				constants.AnnotationMGHSkipAuth: "true",
			},
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: globalhubv1alpha4.DataLayerConfig{
				Kafka: globalhubv1alpha4.KafkaConfig{
					TransportFormat: globalhubv1alpha4.CloudEvents,
				},
				// generate default kafka config
				// Kafka: &globalhubv1alpha4.KafkaConfig{
				// 	TransportFormat: globalhubv1alpha4.CloudEvents,
				// },
				Postgres: globalhubv1alpha4.PostgresConfig{
					Retention: "18m",
				},
			},
		},
	}

	runtimeClient, err := testClients.ControllerRuntimeClient(testOptions.GlobalHub.Name, scheme)
	Expect(err).ShouldNot(HaveOccurred())

	err = runtimeClient.Create(context.TODO(), mcgh)
	if !errors.IsAlreadyExists(err) {
		Expect(err).ShouldNot(HaveOccurred())
	}

	By("Verifying the multicluster-global-hub-grafana/manager")
	Eventually(func() error {
		err := checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-operator")
		if err != nil {
			return err
		}
		err = checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-manager")
		if err != nil {
			return err
		}
		return checkDeployAvailable(runtimeClient, Namespace, "multicluster-global-hub-grafana")
	}, 3*time.Minute, 1*time.Second).Should(Succeed())
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
