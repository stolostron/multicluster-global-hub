package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
)

func createGlobalHubCR() error {
	By("Creating client for the hub cluster")
	config, err := clientcmd.BuildConfigFromFlags("", testOptions.HubCluster.KubeConfig)
	if err != nil {
		Expect(err).ShouldNot(HaveOccurred())
	}

	scheme := runtime.NewScheme()
	operatorv1alpha3.AddToScheme(scheme)
	runtimeClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	Expect(err).ShouldNot(HaveOccurred())

	By("Deploying operator with builded image")
	err = replaceContent(rootDir+"/operator/config/manager/manager.yaml",
		"quay.io/stolostron/multicluster-global-hub-manager:latest", testOptions.HubCluster.ManagerImageREF)
	Expect(err).NotTo(HaveOccurred())

	err = replaceContent(rootDir+"/operator/config/manager/manager.yaml",
		"quay.io/stolostron/multicluster-global-hub-agent:latest", testOptions.HubCluster.AgentImageREF)
	Expect(err).NotTo(HaveOccurred())

	cmd := exec.Command("make", "-C", "../../../operator", "deploy")
	setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
	setCommandEnv(cmd, "IMG", testOptions.HubCluster.OperatorImageREF)
	output, err := cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred())
	fmt.Println(string(output))

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
	Expect(err).ShouldNot(HaveOccurred())

	By("Verifying the multicluster-global-hub-grafana/manager")
	Eventually(func() error {
		err := deploymentReady(runtimeClient, "open-cluster-management", "multicluster-global-hub-operator")
		if err != nil {
			return err
		}
		err = deploymentReady(runtimeClient, "open-cluster-management", "multicluster-global-hub-manager")
		if err != nil {
			return err
		}
		return deploymentReady(runtimeClient, "open-cluster-management", "multicluster-global-hub-grafana")
	}, 3*time.Minute, 2*time.Second).Should(Succeed())

	// globalhub setup for e2e
	if os.Getenv("IS_CANARY_ENV") != "true" {
		By("deploy globalbub for e2e ENV")
		namespace := "open-cluster-management"

		By("updating deployment && cluster-manager")
		cmd := exec.Command("kubectl", "patch", "deployment", "governance-policy-propagator", "-n", "open-cluster-management", "-p", "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-propagator\",\"image\":\"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0\"}]}}}}")
		setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
		err := cmd.Run()
		Expect(err).Should(Succeed())

		cmd = exec.Command("kubectl", "patch", "clustermanager", "cluster-manager", "--type", "merge", "-p", "{\"spec\":{\"placementImagePullSpec\":\"quay.io/open-cluster-management/placement:latest\"}}")
		setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
		err = cmd.Run()
		Expect(err).Should(Succeed())

		cmd = exec.Command("kubectl", "apply", "-f", fmt.Sprintf("%s/test/setup/hoh/components/manager-service-local.yaml", rootDir), "-n", namespace)
		setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
		err = cmd.Run()
		Expect(err).Should(Succeed())

		Eventually(func() error {
			cmd = exec.Command("kubectl", "annotate", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "service.beta.openshift.io/inject-cabundle-")
			setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return err
			}
			fmt.Println(string(output))

			cmd = exec.Command("kubectl", "get", "secret", "multicluster-global-hub-webhook-certs", "-n", namespace, "-o", "jsonpath={.data.tls\\.crt}")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
			ca, _ := cmd.Output()
			// fmt.Println(string(ca))

			cmd = exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "-n", namespace, "-p", fmt.Sprintf("{\"webhooks\":[{\"name\":\"global-hub.open-cluster-management.io\",\"clientConfig\":{\"caBundle\":\"%s\"}}]}", ca))
			setCommandEnv(cmd, "KUBECONFIG", testOptions.HubCluster.KubeConfig)
			output, err = cmd.CombinedOutput()
			if err != nil {
				return err
			}
			fmt.Println(string(output))

			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	}

	return nil
}

func setCommandEnv(cmd *exec.Cmd, key, val string) {
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
}

func replaceContent(file string, old, new string) error {
	cmd := exec.Command("sed", "-i", fmt.Sprintf("s|%s|%s|g", old, new), file)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	return err
}

func deploymentReady(runtimeClient client.Client, namespace, name string) error {
	deployment := &appsv1.Deployment{}
	err := runtimeClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return err
	}
	if deployment.Status.UnavailableReplicas > 0 {
		return nil
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		fmt.Printf("deployment image: %s/%s: %s\n", deployment.Name, container.Name, container.Image)
	}
	return fmt.Errorf("deployment: %s is not ready", deployment.Name)
}
