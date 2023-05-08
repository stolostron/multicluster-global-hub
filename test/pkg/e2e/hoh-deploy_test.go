package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
	"io/ioutil"
	"bytes"
	"strings"
	"regexp"
	
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"	
)

func deployGlobalHub() error {
	currentDir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	rootDir := fmt.Sprintf("%s/../../..", currentDir)

	kubeconfig := ""

	if os.Getenv("IS_CANARY_ENV") == "true" {
		kubeconfig = testOptions.HubCluster.KubeConfig
	} else {
		kubeconfig = rootDir+"/test/setup/config/kubeconfig"
		testOptions.HubCluster.KubeConfig = kubeconfig
		testOptionsContainer.Options.HubCluster.KubeConfig = kubeconfig
	}
	fmt.Println(kubeconfig)

	// Create the dynamic client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		Expect(err).ShouldNot(HaveOccurred())
	}
	dynClient, err := dynamic.NewForConfig(config)
	Expect(err).ShouldNot(HaveOccurred())

	if os.Getenv("IS_CANARY_ENV") != "true" {
		By("deploy globalbub for e2e ENV")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "--context", "kind-hub1", "apply", "-f", fmt.Sprintf("%s/pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml", rootDir))
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			output, err := cmd.CombinedOutput()
			if err == nil {
				fmt.Println(string(output))
			} else {
				fmt.Println(string(output))
				return err
			}

			cmd = exec.Command("kubectl", "--context", "kind-hub2", "apply", "-f", fmt.Sprintf("%s/pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml", rootDir))
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			output, err = cmd.CombinedOutput()
			if err == nil {
				fmt.Println(string(output))
			} else {
				fmt.Println(string(output))
				return err
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	}

	By("checking postgresql is ready")
	if os.Getenv("IS_CANARY_ENV") == "true" {
		postgrePath := "../../../operator/config/samples/storage/deploy_postgres.sh"
		fmt.Println(postgrePath)
		cmd := exec.Command("/bin/bash", postgrePath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		_, err = cmd.Output()
		Expect(err).ShouldNot(HaveOccurred())
	} else {
		postgrePath := rootDir+"/test/setup/hoh/postgres_setup.sh"
		fmt.Println(postgrePath)
		cmd := exec.Command("/bin/bash", postgrePath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", rootDir+"/test/setup/config/kubeconfig-hub-microshift"))
		output, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			return err
		}
	}
	
	Eventually(func() error {
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
		proxyReadyReplicas, proxyOk := postgresClusterMap["status"].(map[string]interface{})["proxy"].(map[string]interface{})["pgBouncer"].(map[string]interface{})["readyReplicas"].(int64)
		instances := postgresClusterMap["status"].(map[string]interface{})["instances"].([]interface{})
		instanceReadyReplicas := int64(0)
		for _, instance := range instances {
			instanceMap := instance.(map[string]interface{})
			if instanceMap["readyReplicas"] != nil && instanceMap["readyReplicas"].(int64) > 0 {
				instanceReadyReplicas++
			}
		}
		if proxyOk && proxyReadyReplicas > 0 && instanceReadyReplicas > 0{
			return nil
		}
		return fmt.Errorf("postgres is not ready")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	Eventually(func() error {
		// get databaseUri and add into options
		containerPgPort := "32432"
		cmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", "hub-of-hubs")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}

		containerNodeIP := strings.TrimSpace(string(output))
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			return err
		}
		fmt.Println("Container Node IP:", containerNodeIP)

		// Execute kubectl command to get secret value
		cmd = exec.Command("bash", "-c", fmt.Sprintf("kubectl get secret storage-secret -n open-cluster-management --kubeconfig %s/test/resources/kubeconfig/kubeconfig-hub-of-hubs -ojsonpath='{.data.database_uri}' | base64 -d", rootDir))
		output, err = cmd.CombinedOutput()
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			return err
		}

		databaseUri := strings.TrimSpace(string(output))
		// Replace container node IP and port in database URI
		containerPgURI := strings.Replace(databaseUri, "@.*hoh", fmt.Sprintf("@%s:%s/hoh", containerNodeIP, containerPgPort), -1)
		
		pattern := `@.*hoh`
		replacement := fmt.Sprintf("@%s:%s/hoh", containerNodeIP, containerPgPort)
		re := regexp.MustCompile(pattern)
		modifiedURI := re.ReplaceAllString(containerPgURI, replacement)
		fmt.Println(modifiedURI)
		// add DatabaseURI in options
		testOptionsContainer.Options.HubCluster.DatabaseURI = modifiedURI
		testOptions.HubCluster.DatabaseURI = modifiedURI

		return nil
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	By("checking kafka is ready")
	Eventually(func() error {
		if os.Getenv("IS_CANARY_ENV") == "true" {
			kafkaPath := "../../../operator/config/samples/transport/deploy_kafka.sh"
			fmt.Println(kafkaPath)
			cmd := exec.Command("/bin/bash", kafkaPath)
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			_, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("Error deploying kafka: %s", err)
			}
		} else {
			kafkaPath := rootDir+"/test/setup/hoh/kafka_setup.sh"
			fmt.Println(kafkaPath)
			cmd := exec.Command("/bin/bash", kafkaPath)
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", rootDir+"/test/setup/config/kubeconfig-hub-microshift"))
			output, err := cmd.CombinedOutput()
			if err == nil {
				fmt.Println(string(output))
			} else {
				fmt.Println(string(output))
				return err
			}
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
		if !ok && len(conditions) == 0{
			return fmt.Errorf("Failed to extract conditions from Kafka object")
		}
		// check the status conditions for the Kafka cluster.
		for _, c := range conditions {
			condition := c.(map[string]interface{})
			if condition["type"].(string) == "Ready" && condition["status"].(string) == "True" {
				return nil
			}
		}
		return fmt.Errorf("The condition from kafka object is not ready")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
	
	// wait deployment is ready
	// check global hub operator / pod is running
	By("deploying operator")
	Eventually(func() error {
		multiClusterGlobalHubManagerImageRef := os.Getenv("MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF")
		if multiClusterGlobalHubManagerImageRef != "" {
			cmd := exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-manager:latest|%s|g", multiClusterGlobalHubManagerImageRef),rootDir+"/operator/config/manager/manager.yaml")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			cmd.Run()
		}
		multiClusterGlobalHubAgentImageRef := os.Getenv("MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF")
		if multiClusterGlobalHubAgentImageRef != "" {
			cmd := exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-agent:latest|%s|g", multiClusterGlobalHubAgentImageRef), rootDir+"/operator/config/manager/manager.yaml")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			cmd.Run()
		}

		cmd := exec.Command("make", "-C", "../../../operator", "deploy")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		err = cmd.Run()
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

		// transferring Unstructured into Map
		deployMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
		if err != nil {
			return err
		}
		// get and check status.proxy.readyReplicas and status.instances.readyReplicas larget than zero
		readyReplicas, ok := deployMap["status"].(map[string]interface{})["readyReplicas"].(int64)
		if ok && readyReplicas > 0{
			return nil
		}
		return fmt.Errorf("postgres is not ready")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
	
	// deploy CR by unstructured.Unstructured
	By("deploying CR")
	transportSecretName := "transport-secret"
	storageSecretName := "storage-secret"

	yamlFile, err := ioutil.ReadFile(fmt.Sprintf("%s/test/setup/hoh/components/mgh-v1alpha2-cr.yaml", rootDir))
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
		if err != nil || podList == nil{
			return fmt.Errorf("failed to list Pods: %v\n", err)
		}
		
		expectResCount := 2
		for _, pod := range podList.Items {
			fmt.Println(pod.GetLabels()["name"])
			if pod.GetLabels()["name"] == grafanaPodName || pod.GetLabels()["name"] == managerPodName {
				podData := pod.Object
				var podObj corev1.Pod
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(podData, &podObj)
				if err != nil {
					return fmt.Errorf("failed to convert Pod data to object: %v\n", err)
				}
				if podObj.Status.Phase != "Running" {
					return fmt.Errorf("multicluster global hub is not running")
				}
				expectResCount -= 1
			}
		}
		if expectResCount != 0 {
			return fmt.Errorf("deploy multicluster-global-hub-grafana/manager failed")
		}
		return nil
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
	
	// globalhub setup for e2e
	if os.Getenv("IS_CANARY_ENV") != "true" {
		By("deploy globalbub for e2e ENV")
		namespace := "open-cluster-management"	

		By("updating deployment && cluster-manager")
		cmd := exec.Command("kubectl", "patch", "deployment", "governance-policy-propagator", "-n", "open-cluster-management", "-p", "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-propagator\",\"image\":\"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0\"}]}}}}")		// cmd.Run()
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		output, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			Expect(err).Should(Succeed())
		}

		cmd = exec.Command("kubectl", "patch", "clustermanager", "cluster-manager", "--type", "merge", "-p", "{\"spec\":{\"placementImagePullSpec\":\"quay.io/open-cluster-management/placement:latest\"}}")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		output, err = cmd.CombinedOutput()
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			Expect(err).Should(Succeed())
		}

		cmd = exec.Command("kubectl", "apply", "-f" ,fmt.Sprintf("%s/test/setup/hoh/components/manager-service-local.yaml", rootDir), "-n", namespace)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
		output, err = cmd.CombinedOutput()
		if err == nil {
			fmt.Println(string(output))
		} else {
			fmt.Println(string(output))
			Expect(err).Should(Succeed())
		}

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
			if !ok && len(conditions) == 0{
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

		Eventually(func() error {
			cmd = exec.Command("kubectl", "annotate", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "service.beta.openshift.io/inject-cabundle-")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			output, err = cmd.CombinedOutput()
			if err == nil {
				fmt.Println(string(output))
			} else {
				fmt.Println(string(output))
				return err
			}

			cmd = exec.Command("kubectl", "get", "secret", "multicluster-global-hub-webhook-certs", "-n", namespace, "-o", "jsonpath={.data.tls\\.crt}")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			ca, _ := cmd.Output()
			fmt.Println(string(ca))

			cmd = exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "-n", namespace, "-p", fmt.Sprintf("{\"webhooks\":[{\"name\":\"global-hub.open-cluster-management.io\",\"clientConfig\":{\"caBundle\":\"%s\"}}]}", ca))
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
			output, err = cmd.CombinedOutput()
			if err == nil {
				fmt.Println(string(output))
			} else {
				fmt.Println(string(output))
				return err
			}

			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	}
	return nil
}	
