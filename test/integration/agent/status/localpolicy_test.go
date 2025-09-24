package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/agent/status -v -ginkgo.focus "LocalPolicyEmitters"
var _ = Describe("LocalPolicyEmitters", Ordered, func() {
	var localRootPolicy *policyv1.Policy

	It("be able to sync policy spec", func() {
		By("Create a root policy")
		localRootPolicy = &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "root-policy-test123",
				Namespace:  "default",
				Finalizers: []string{constants.GlobalHubCleanupFinalizer},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        true,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
			Status: policyv1.PolicyStatus{},
		}
		Expect(runtimeClient.Create(ctx, localRootPolicy)).NotTo(HaveOccurred())

		By("Check the local policy spec can be read from cloudevents consumer")
		fmt.Println("============================ create policy -> policy spec event: disabled")
		Eventually(func() error {
			evt := receivedEvents[string(enum.LocalPolicySpecType)]
			if evt == nil {
				return fmt.Errorf("not get the event: %s", string(enum.LocalPolicySpecType))
			}
			fmt.Println(evt)
			bundle := &generic.GenericBundle[policyv1.Policy]{}
			err := evt.DataAs(bundle)
			if err != nil {
				return err
			}
			for _, policy := range bundle.Update {
				if policy.Name == "root-policy-test123" {
					if policy.Spec.Disabled {
						return nil
					}
				}
			}
			return fmt.Errorf("want %v, got %v", string(enum.LocalPolicySpecType), evt.Type())
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		By("Update the root policy")
		err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(localRootPolicy), localRootPolicy)
		Expect(err).Should(Succeed())

		localRootPolicy.Spec.Disabled = false
		err = runtimeClient.Update(ctx, localRootPolicy)
		Expect(err).Should(Succeed())

		By("Check the local policy spec can be read from cloudevents consumer")
		fmt.Println("============================ update policy -> policy spec event: enabled")
		Eventually(func() error {
			evt := receivedEvents[string(enum.LocalPolicySpecType)]
			if evt == nil {
				return fmt.Errorf("not get the event: %s", string(enum.LocalPolicySpecType))
			}
			bundle := generic.GenericBundle[policyv1.Policy]{}
			err := evt.DataAs(&bundle)
			if err != nil {
				return err
			}
			for _, policy := range bundle.Update {
				if policy.Name == "root-policy-test123" {
					if !policy.Spec.Disabled {
						return nil
					}
				}
			}
			return fmt.Errorf("policy should be enabled")
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("be able to sync local policy compliance", func() {
		By("Create the compliance on the root policy status")
		localRootPolicy.Status = policyv1.PolicyStatus{
			ComplianceState: policyv1.Compliant,
			Status: []*policyv1.CompliancePerClusterStatus{
				{
					ClusterName:      "policy-cluster1",
					ClusterNamespace: "policy-cluster1",
					ComplianceState:  policyv1.Compliant,
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, localRootPolicy)).Should(Succeed())

		By("Check the compliance can be read from cloudevents consumer")
		Eventually(func() error {
			evt := receivedEvents[string(enum.LocalComplianceType)]
			if evt == nil {
				return fmt.Errorf("not get the event: %s", string(enum.LocalComplianceType))
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("be able to sync local policy complete compliance", func() {
		By("Create the compliance on the root policy status")
		localRootPolicy.Status = policyv1.PolicyStatus{
			ComplianceState: policyv1.Compliant,
			Status: []*policyv1.CompliancePerClusterStatus{
				{
					ClusterName:      "policy-cluster1",
					ClusterNamespace: "policy-cluster1",
					ComplianceState:  policyv1.NonCompliant,
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, localRootPolicy)).Should(Succeed())

		By("Check the complete compliance can be read from cloudevents consumer")
		Eventually(func() error {
			evt := receivedEvents[string(enum.LocalCompleteComplianceType)]
			if evt == nil {
				return fmt.Errorf("not get the event: %s", string(enum.LocalCompleteComplianceType))
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("be able to sync replicated policy event", func() {
		By("Create namespace and cluster for the replicated policy")
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-cluster1",
			},
		}, &client.CreateOptions{})
		Expect(err).Should(Succeed())

		By("Create the cluster")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "policy-cluster1",
			},
		}
		Expect(runtimeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  constants.ClusterIdClaimName,
					Value: "3f406177-34b2-4852-88dd-ff2809680336",
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, cluster)).Should(Succeed())

		By("Create the replicated policy")
		replicatedPolicyName := fmt.Sprintf("%s.%s", localRootPolicy.Namespace, localRootPolicy.Name)
		replicatedPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      replicatedPolicyName,
				Namespace: "policy-cluster1",
				Labels: map[string]string{
					constants.PolicyEventRootPolicyNameLabelKey: replicatedPolicyName,
					constants.PolicyEventClusterNameLabelKey:    "policy-cluster1",
				},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        false,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
		}
		Expect(runtimeClient.Create(ctx, replicatedPolicy)).ToNot(HaveOccurred())

		By("Create the replicated policy event")
		replicatedPolicy.Status = policyv1.PolicyStatus{
			ComplianceState: policyv1.NonCompliant,
			Details: []*policyv1.DetailsPerTemplate{
				{
					TemplateMeta: metav1.ObjectMeta{
						Name: "test-local-policy-template",
					},
					History: []policyv1.ComplianceHistory{
						{
							EventName:     "default.root-policy-test123.17b0db2427432200",
							LastTimestamp: metav1.Now(),
							Message: `NonCompliant; violation - limitranges [container-mem-limit-range] not found in namespace
							default`,
						},
					},
				},
			},
		}
		Expect(runtimeClient.Status().Update(ctx, replicatedPolicy)).Should(Succeed())

		By("Check the local replicated policy event can be read from cloudevents consumer")
		Eventually(func() error {
			evt := receivedEvents[string(enum.LocalReplicatedPolicyEventType)]
			if evt == nil {
				return fmt.Errorf("not get the event: %s", string(enum.LocalReplicatedPolicyEventType))
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})
