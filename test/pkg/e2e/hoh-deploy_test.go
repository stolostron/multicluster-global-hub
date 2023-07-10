package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
	"strings"
	"regexp"

	// "encoding/base64"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"	
	// "github.com/stolostron/multicluster-global-hub/pkg/database"
)

func createGlobalHubCR() error {
	fmt.Println(testOptions.HubCluster.KubeConfig)

	// Create the dynamic client
	config, err := clientcmd.BuildConfigFromFlags("", testOptions.HubCluster.KubeConfig)
	if err != nil {
		Expect(err).ShouldNot(HaveOccurred())
	}
	dynClient, err := dynamic.NewForConfig(config)
	Expect(err).ShouldNot(HaveOccurred())

	clientset, err := kubernetes.NewForConfig(config)
	Expect(err).ShouldNot(HaveOccurred())

	if os.Getenv("IS_CANARY_ENV") != "true" {
		By("deploy globalbub for e2e ENV")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("%s/pkg/testdata/crds/0000_00_agent.open-cluster-management.io_klusterletaddonconfigs_crd.yaml", rootDir))
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
			output, err := cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				return err
			}

			for _, managedCluster := range testOptions.ManagedClusters {
				if managedCluster.Name != managedCluster.LeafHubName {
					cmd = exec.Command("kubectl", "--context", managedCluster.Name, "apply", "-f", testOptions.HubCluster.CrdsDir, "--validate=false")
					cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
					output, err = cmd.CombinedOutput()
					fmt.Println(string(output))
					if err != nil {
						return err
					}
				}
			}
			return nil 
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	}

	By("checking postgresql is ready")
	cmd := exec.Command("/bin/bash", testOptions.HubCluster.StoragePath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	Expect(err).ShouldNot(HaveOccurred())
	
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

	By("checking kafka is ready")
	Eventually(func() error {
		cmd := exec.Command("/bin/bash", testOptions.HubCluster.TransportPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			Expect(err).ShouldNot(HaveOccurred())
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
	if testOptions.HubCluster.ManagerImageREF != "" {
		cmd := exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-manager:latest|%s|g", testOptions.HubCluster.ManagerImageREF), rootDir+"/operator/config/manager/manager.yaml")
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			fmt.Println(err)
			Expect(err).NotTo(HaveOccurred())
		}
	}
	if testOptions.HubCluster.AgentImageREF != "" {
		cmd := exec.Command("sed", "-i", fmt.Sprintf("s|quay.io/stolostron/multicluster-global-hub-agent:latest|%s|g", testOptions.HubCluster.AgentImageREF), rootDir+"/operator/config/manager/manager.yaml")
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			fmt.Println(err)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	cmd = exec.Command("make", "-C", "../../../operator", "deploy")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		fmt.Println(err)
		Expect(fmt.Errorf("failed to execute make deploy-operator: %v\n", err)).NotTo(HaveOccurred())
	}

	Eventually(func() error {
		deploymentList, err := clientset.AppsV1().Deployments("open-cluster-management").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			fmt.Println(err)
			return err
		}

		for _, deployment := range deploymentList.Items {
			if deployment.Labels["name"] == "multicluster-global-hub-operator" {
				if deployment.Status.ReadyReplicas > 0 {
					return nil
				}
			}
		}
		return fmt.Errorf("postgres is not ready")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	// deploy CR by unstructured.Unstructured
	By("deploying CR")
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operator.open-cluster-management.io/v1alpha3",
			"kind":       "MulticlusterGlobalHub",
			"metadata": map[string]interface{}{
				"name": "multiclusterglobalhub",
			},
			"spec": map[string]interface{}{
				"dataLayer": map[string]interface{}{
					"type": "largeScale",
					"largeScale": map[string]interface{}{
						"kafka": map[string]interface{}{
							// "name": transportSecretName,
							"transportFormat": "cloudEvents",
						},
						// "postgres": map[string]interface{}{
						// 	"name": storageSecretName,
						// },
					},
				},
			},
		},
	}
	_, err = dynClient.Resource(
		schema.GroupVersionResource{
			// apis Gourpversion
			Group:    "operator.open-cluster-management.io",
			Version:  "v1alpha3",
			Resource: "multiclusterglobalhubs",
		},
	).Namespace("open-cluster-management").Create(context.TODO(), resource, metav1.CreateOptions{})
	
	Eventually(func() error {
		grafanaPodName := "multicluster-global-hub-grafana"
		managerPodName := "multicluster-global-hub-manager"

		deploymentList, err := clientset.AppsV1().Deployments("open-cluster-management").List(context.Background(), metav1.ListOptions{
			LabelSelector: labels.Everything().String(),
		})
		if err != nil || deploymentList == nil {
			return fmt.Errorf("failed to list Pods: %v\n", err)
		}
		
		expectResCount := 2
		for _, deployment := range deploymentList.Items {
			if deployment.Labels["name"] == grafanaPodName || deployment.Labels["name"] == managerPodName {
				if deployment.Status.UnavailableReplicas != 0 {
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
		cmd := exec.Command("kubectl", "patch", "deployment", "governance-policy-propagator", "-n", "open-cluster-management", "-p", "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-propagator\",\"image\":\"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0\"}]}}}}")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
		err := cmd.Run()
		if err != nil {
			Expect(err).Should(Succeed())
		}

		cmd = exec.Command("kubectl", "patch", "clustermanager", "cluster-manager", "--type", "merge", "-p", "{\"spec\":{\"placementImagePullSpec\":\"quay.io/open-cluster-management/placement:latest\"}}")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
		err = cmd.Run()
		if err != nil {
			Expect(err).Should(Succeed())
		}

		cmd = exec.Command("kubectl", "apply", "-f" ,fmt.Sprintf("%s/test/setup/hoh/components/manager-service-local.yaml", rootDir), "-n", namespace)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
		err = cmd.Run()
		if err != nil {
			Expect(err).Should(Succeed())
		}

		Eventually(func() error {
			cmd = exec.Command("kubectl", "annotate", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "service.beta.openshift.io/inject-cabundle-")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return err
			}

			cmd = exec.Command("kubectl", "get", "secret", "multicluster-global-hub-webhook-certs", "-n", namespace, "-o", "jsonpath={.data.tls\\.crt}")
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
			ca, _ := cmd.Output()
			// fmt.Println(string(ca))

			cmd = exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "multicluster-global-hub-mutator", "-n", namespace, "-p", fmt.Sprintf("{\"webhooks\":[{\"name\":\"global-hub.open-cluster-management.io\",\"clientConfig\":{\"caBundle\":\"%s\"}}]}", ca))
			cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", testOptions.HubCluster.KubeConfig))
			output, err = cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				return err 
			}

			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	}

	By("deploying operator")
	Eventually(func() error {
		postgresSecret, err := clientset.CoreV1().Secrets("open-cluster-management").Get(context.Background(), "multicluster-global-hub-storage", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get postgre secret failed: %v", err)
		}
		
		databaseURI := postgresSecret.Data["database_uri"]

		fmt.Println("Decoded database_uri:", string(databaseURI))

		containerPgURI := strings.Replace(string(databaseURI), "@.*hoh", fmt.Sprintf("@%v:%d/hoh", testOptions.HubCluster.DatabaseExternalHost, testOptions.HubCluster.DatabaseExternalPort), -1)
		
		pattern := `@.*hoh`
		replacement := fmt.Sprintf("@%s:%d/hoh", testOptions.HubCluster.DatabaseExternalHost, testOptions.HubCluster.DatabaseExternalPort)
		re := regexp.MustCompile(pattern)
		modifiedURI := re.ReplaceAllString(containerPgURI, replacement)
		fmt.Printf("\n modifiedURI: \n %v\n", modifiedURI)
		// add DatabaseURI in options
		testOptions.HubCluster.DatabaseURI = modifiedURI

		return nil
	}, 1*time.Minute, 1*time.Second).Should(Succeed())

	return nil
}	
