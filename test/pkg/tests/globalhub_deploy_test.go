package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
	"io/ioutil"
	"bytes"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

func deployGlobalHub() error {
	currentDir, _ := os.Getwd()
	rootDir := fmt.Sprintf("%s/../../..", currentDir)
	kubeconfig := "../config/kubeconfig"

	var postgrePath string
	var kafkaPath string

	// Load the kubeconfig file.
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		Expect(err).ShouldNot(HaveOccurred())
	}

	// Create the dynamic client.
	dynClient, err := dynamic.NewForConfig(config)
	Expect(err).ShouldNot(HaveOccurred())

	By("checking postgresql is ready")
	Eventually(func() error {
		if os.Getenv("IS_CANARY_ENV") == "true" {
			postgrePath = "../../../operator/config/samples/storage/deploy_postgres.sh"
		} else {
			postgrePath = "../../setup/hoh/postgres_setup.sh"
		}
		cmd := exec.Command("/bin/bash", postgrePath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		_, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("Error deploying postgre: %s", err)
		}

		// check postgres is ready by checking proxyReadyReplicas && proxyReadyReplicas
		name := "hoh"
		namespace := "hoh-postgres"

		postgresCluster, err := dynClient.Resource(
			schema.GroupVersionResource{
				Group:    "postgres-operator.crunchydata.com",
				Version:  "v1beta1",
				Resource: "postgresclusters",
			},
		).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("Postgres cluster %s/%s not found\n", namespace, name)
			}
			return err
		}

		// transferring Unstructured into Map
		postgresClusterMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(postgresCluster)
		if err != nil {
			return err
		}

		// get and check status.proxy.readyReplicas and status.instances.readyReplicas larget than zero
		proxyReadyReplicas := postgresClusterMap["status"].(map[string]interface{})["proxy"].(map[string]interface{})["pgBouncer"].(map[string]interface{})["readyReplicas"].(int64)
		instanceReadyReplicas := postgresClusterMap["status"].(map[string]interface{})["instances"].([]interface{})[0].(map[string]interface{})["readyReplicas"].(int64)

		if proxyReadyReplicas > 0 && instanceReadyReplicas > 0 {
			return nil
		}
		return fmt.Errorf("Postgres cluster deployment failed.")
	}, 1*time.Minute, 1*time.Second).Should(Succeed())

	By("checking kafka is ready")
	Eventually(func() error {
		if os.Getenv("IS_CANARY_ENV") == "true" {
			kafkaPath = "../../../operator/config/samples/transport/deploy_kafka.sh"
		} else {
			kafkaPath = "../../setup/hoh/kafka_setup.sh"
		}

		cmd := exec.Command("/bin/bash", kafkaPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		_, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("Error deploying kafka: %s", err)
		}

		// check kafka is ready by checking condition status
		namespace := "kafka"
		name := "kafka-brokers-cluster"

		kafka, err := dynClient.Resource(
			schema.GroupVersionResource{
				Group:    "kafka.strimzi.io",
				Version:  "v1beta2",
				Resource: "kafkas",
			},
		).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("Kafka cluster %s/%s not found\n", namespace, name)
			} else {
				return err
			}
		}

		conditions, ok := kafka.Object["status"].(map[string]interface{})["conditions"].([]interface{})
		if !ok {
			return fmt.Errorf("Failed to extract conditions from Kafka object")
		}
		// check the status conditions for the Kafka cluster.
		for _, c := range conditions {
			condition := c.(map[string]interface{})
			if condition["type"].(string) == "Ready" && condition["status"].(string) == "True" {
				fmt.Println("kafka condition is ok")
				return nil
			}
		}
		return fmt.Errorf("The condition from kafka object is not ready")
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
	
	// wait deployment is ready
	// check global hub operator / pod is running
	By("deploying operator")
	Eventually(func() error {
		cmd := exec.Command("make", "deploy-operator")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		cmd.Dir = "../../../operator"
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to execute make deploy-operator: %v\n", err)
		}
	
		// check GlobalHub Operator status
		deployment, err := dynClient.Resource(
			schema.GroupVersionResource{
				Group: "apps",
				Version: "v1",
				Resource: "deployments",
			},
		).Namespace("open-cluster-management").Get(context.Background(),  "multicluster-global-hub-operator", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("GlobalHub Operator is not running: %v\n", err)
		}

		conditions, ok := deployment.Object["status"].(map[string]interface{})["conditions"].([]interface{})
		if !ok {
			return fmt.Errorf("Failed to extract conditions from Kafka object")
		}
		for _, c := range conditions {
			condition := c.(map[string]interface{})
			if condition["status"].(string) != "True" {
				return fmt.Errorf("operator is not running")
			}
		}
		return nil
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
	
	// deploy CR by unstructured.Unstructured
	By("deploying CR")
	It("deploying CR", func() {
		transportSecretName := "transport-secret"
		storageSecretName := "storage-secret"

		yamlFile, err := ioutil.ReadFile("mgh-v1alpha2-cr.yaml")
		Expect(err).NotTo(HaveOccurred())
		decoder := yaml.NewYAMLOrJSONDecoder(ioutil.NopCloser(bytes.NewReader(yamlFile)), 4096)
		unstructuredObj := &unstructured.Unstructured{}
		err = decoder.Decode(unstructuredObj)
		Expect(err).NotTo(HaveOccurred())

		// Replace actual secret names
		obj := unstructuredObj.Object
		unstructured.SetNestedField(obj, transportSecretName, "spec", "dataLayer", "largeScale", "kafka", "name")
		unstructured.SetNestedField(obj, storageSecretName, "spec", "dataLayer", "largeScale", "postgres", "name")

		_, err = dynClient.Resource(
			schema.GroupVersionResource{
				Group:    "operator.open-cluster-management.io",
				Version:  "v1alpha2",
				Resource: "multiclusterglobalhubs",
			},
		).Namespace("open-cluster-management").Create(context.TODO(), unstructuredObj, metav1.CreateOptions{})
	})
	
	Eventually(func() error {
		namespace := "open-cluster-management"
		grafanaPodName := "multicluster-global-hub-grafana"
		managerPodName := "multicluster-global-hub-manager"
	
		podList, err := dynClient.Resource(
			schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
		).Namespace(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labels.Everything().String(),
		})
		if err != nil {
			return fmt.Errorf("failed to list Pods: %v\n", err)
		}
	
		for _, pod := range podList.Items {
			if pod.GetNamespace() == namespace && (pod.GetLabels()["app.kubernetes.io/name"] == grafanaPodName ||pod.GetLabels()["app.kubernetes.io/name"] == managerPodName ) {
				podData := pod.Object
				var podObj corev1.Pod
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(podData, &podObj)
				if err != nil {
					return fmt.Errorf("failed to convert Pod data to object: %v\n", err)
				}
				if podObj.Status.Phase != "Running" {
					return fmt.Errorf("multicluster global hub is not running")
				}
			}
		}
		return nil
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
	
	// globalhub setup for e2e
	if os.Getenv("IS_CANARY_ENV") != "true" {
		registry := os.Getenv("REGISTRY")
		if registry == "" {
			registry = "quay.io/stolostron"
		}

		openShiftCI := os.Getenv("OPENSHIFT_CI")
		if openShiftCI == "" {
			openShiftCI = "false"
		}

		tag := os.Getenv("TAG")
		if tag == "" {
			tag = "latest"
		}

		if openShiftCI == "false" {
			multiClusterGlobalHubManagerImageRef := os.Getenv("MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF")
			if multiClusterGlobalHubManagerImageRef == "" {
				multiClusterGlobalHubManagerImageRef = fmt.Sprintf("%s/multicluster-global-hub-manager:%s", registry, tag)
			}

			multiClusterGlobalHubAgentImageRef := os.Getenv("MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF")
			if multiClusterGlobalHubAgentImageRef == "" {
				multiClusterGlobalHubAgentImageRef = fmt.Sprintf("%s/multicluster-global-hub-agent:%s", registry, tag)
			}

			multiClusterGlobalHubOperatorImageRef := os.Getenv("MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF")
			if multiClusterGlobalHubOperatorImageRef == "" {
				multiClusterGlobalHubOperatorImageRef = fmt.Sprintf("%s/multicluster-global-hub-operator:%s", registry, tag)
			}
		}

		namespace := "open-cluster-management"

		It("should deploy multicluster-global-hub-manager", func() {
			cmd := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("%s/components/leader-election-configmap.yaml", currentDir), "-n", namespace)
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), err)
		
			cmd = exec.Command("kubectl", "--context", "kind-hub1", "apply", "-f", fmt.Sprintf("%s/pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml", rootDir))
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred(), err)
		
			cmd = exec.Command("kubectl", "--context", "kind-hub2", "apply", "-f", fmt.Sprintf("%s/pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml", rootDir))
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred(), err)

			cmd = exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-manager:latest|%s|g", rootDir+"/operator/config/manager/manager.yaml"))
			cmd = exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-agent:latest|%s|g", rootDir+"/operator/config/manager/manager.yaml"))
		})

		It("should update HoH images and wait for components to be ready", func() {
			By("updating governance-policy-propagator image")
			command := "kubectl patch deployment governance-policy-propagator -n " + namespace + " -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-propagator\",\"image\":\"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0\"}]}}}}'"
			err := exec.Command(command)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to patch deployment: %v", err))
	
			By("updating cluster-manager placementImagePullSpec")
			command = "kubectl patch clustermanager cluster-manager --type merge -p '{\"spec\":{\"placementImagePullSpec\":\"quay.io/open-cluster-management/placement:latest\"}}'"
			err = exec.Command(command)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to patch clustermanager: %v", err))
	
			By("waiting for core components to be ready")
			Eventually(func() error {
				deployment, err := dynClient.Resource(
					schema.GroupVersionResource{
						Group: "apps",
						Version: "v1",
						Resource: "deployments",
					},
				).Namespace("open-cluster-management-global-hub-system").Get(context.Background(),  "multicluster-global-hub-agent", metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("GlobalHub Agent is not running: %v\n", err)
				}
		
				conditions, ok := deployment.Object["status"].(map[string]interface{})["conditions"].([]interface{})
				if !ok {
					return fmt.Errorf("Failed to extract conditions from Kafka object")
				}
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["status"].(string) != "True" {
						return fmt.Errorf("agent is not running")
					}
				}
				return nil
			})
		})
	}
	return nil
}	


