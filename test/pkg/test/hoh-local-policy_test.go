package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	// "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	// clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
)

const (
	LOCAL_INFORM_POLICY_YAML  = "../../resources/policy/local-inform-limitrange-policy.yaml"
	LOCAL_ENFORCE_POLICY_YAML = "../../resources/policy/local-enforce-limitrange-policy.yaml"

	LOCAL_POLICY_LABEL_KEY   = "local-policy"
	LOCAL_POLICY_LABEL_VALUE = "test"
	LOCAL_POLICY_NAME = "policy-limitrange"
	LOCAL_POLICY_NAMESPACE = "local-policy-namespace"
)

var _ = Describe("Apply local policy to the managed clusters", Ordered, Label("local-policy"), func() {

	var leafHubName string

	BeforeAll(func() {
		leafHubName = clients.LeafHubClusterName()
		localPolicyNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: LOCAL_POLICY_NAMESPACE}}
		clients.KubeClient().CoreV1().Namespaces().Create(context.TODO(), localPolicyNamespace, metav1.CreateOptions{})
		Eventually(func() error {
			_, err := clients.KubeClient().CoreV1().Namespaces().Get(context.TODO(), LOCAL_POLICY_NAMESPACE, metav1.GetOptions{})
			return err
		}, 5 * time.Second, 1 * time.Second).ShouldNot(HaveOccurred())
	})

	It("deploy inform policy to the leaf hub", func() {

		By("Add local policy label to the leaf hub")
		dynamicClient := clients.KubeDynamicClient()
		unstructedObj, err := dynamicClient.Resource(utils.NewManagedClustersGVR()).Get(context.TODO(), leafHubName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		labelMap := unstructedObj.GetLabels()
		labelMap[LOCAL_POLICY_LABEL_KEY] = LOCAL_POLICY_LABEL_VALUE
		unstructedObj.SetLabels(labelMap)

		_, err = dynamicClient.Resource(utils.NewManagedClustersGVR()).Update(context.TODO(), unstructedObj, metav1.UpdateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the local policy label is added to the leaf hub")
		Eventually(func() error {
			unstructedObj, err := dynamicClient.Resource(utils.NewManagedClustersGVR()).Get(context.TODO(), leafHubName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			labelMap = unstructedObj.GetLabels()
			if val, ok := labelMap[LOCAL_POLICY_LABEL_KEY]; ok && val == LOCAL_POLICY_LABEL_VALUE {
				printUnstructedObjLabel(unstructedObj)
				return nil
			}
			return fmt.Errorf("local policy label not found")
		}, 1 * 60 * time.Second).ShouldNot(HaveOccurred())

		By("Deploy the Inform policy to the leaf hub")
		output, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", LOCAL_INFORM_POLICY_YAML)
		klog.V(5).Info(fmt.Sprintf("deploy inform local policy: %s", output))
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the Inform policy is deployed to the leaf hub")
		Eventually(func() error {
			unstrcutedPolicy, err := clients.KubeDynamicClient().Resource(utils.NewPolicyGVR()).Namespace(LOCAL_POLICY_NAMESPACE).Get(context.TODO(), LOCAL_POLICY_NAME, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			unstructedContent := unstrcutedPolicy.UnstructuredContent()
			var policy policiesv1.Policy
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructedContent, &policy)
			Expect(err).ShouldNot(HaveOccurred())
			if len(policy.Status.Status) <=0 {
				return fmt.Errorf("inform local policy status is not ready")
			}
			if (policy.Status.Status[0].ClusterName == leafHubName) && (policy.Status.Status[0].ComplianceState == "NonCompliant") {
				policyStatusStr, _ := json.MarshalIndent(policy.Status, "", "  ")
				klog.V(5).Info(fmt.Sprintf("local PolicyStatus: %s", policyStatusStr))
				return nil
			}
			return fmt.Errorf("local policy is not NonCompliant")
		}, 1 * 60 * time.Second, 5 * time.Second).ShouldNot(HaveOccurred())
	})

	It("deploy enforce policy to the leaf hub", func() {
		output, err := clients.Kubectl(clients.HubClusterName(), "apply", "-f", LOCAL_ENFORCE_POLICY_YAML)
		klog.V(5).Info(fmt.Sprintf("apply enforce local policy: %s", output))
		Expect(err).ShouldNot(HaveOccurred())

		By("Check the enfoce policy is deployed to the leaf hub")
		Eventually(func() error {
			unstrcutedPolicy, err := clients.KubeDynamicClient().Resource(utils.NewPolicyGVR()).Namespace(LOCAL_POLICY_NAMESPACE).Get(context.TODO(), LOCAL_POLICY_NAME, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			unstructedContent := unstrcutedPolicy.UnstructuredContent()
			var policy policiesv1.Policy
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructedContent, &policy)
			Expect(err).ShouldNot(HaveOccurred())
			if len(policy.Status.Status) <=0 {
				return fmt.Errorf("enforce local policy status is not ready")
			}
			if (policy.Status.Status[0].ClusterName == leafHubName) && (policy.Status.Status[0].ComplianceState == "Compliant") {
				policyStatusStr, _ := json.MarshalIndent(policy.Status, "", "  ")
				klog.V(5).Info(fmt.Sprintf("local PolicyStatus: %s", policyStatusStr))
				return nil
			}
			return fmt.Errorf("local policy is not Compliant")
		}, 1 * 60 * time.Second, 5 * time.Second).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Delete the enforced policy")
		deleteInfo, err := clients.Kubectl(clients.HubClusterName(), "delete", "-f", LOCAL_ENFORCE_POLICY_YAML)
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info("delete local policy: ", deleteInfo)

		By("Delete the LimitRange CR from leafhub")
		deleteInfo, err = clients.Kubectl(leafHubName, "delete", "LimitRange", "container-mem-limit-range")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(5).Info(leafHubName, ": ", deleteInfo)

		By("Check the policy is deleted from leafhub")
		Eventually(func() error {
			_, err = clients.KubeDynamicClient().Resource(utils.NewPolicyGVR()).Namespace(LOCAL_POLICY_NAMESPACE).Get(context.TODO(), LOCAL_POLICY_NAME, metav1.GetOptions{})
			Expect(err).Should(HaveOccurred())
			if errors.IsNotFound(err) {
				klog.V(5).Info(fmt.Sprintf("local policy(%s) is deleted from namespace(%s)!", LOCAL_POLICY_NAME, LOCAL_POLICY_NAMESPACE))
				return nil
			}
			return fmt.Errorf("local policy is not deleted")
		}, 1 * 60 * time.Second, 5 * time.Second).ShouldNot(HaveOccurred())

	})
})

func printUnstructedObjLabel(obj *unstructured.Unstructured) {
	for k, v := range obj.GetLabels() {
		klog.V(5).Info(fmt.Sprintf("Object(%s): %s -> %s", obj.GetName(), k, v))
	}
}
