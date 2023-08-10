package grc

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestClusterPerPolicyStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", 1, extractObjIDFunc)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	bundle.UpdateObject(policy) // add obj to bundle
	version := bundle.GetBundleVersion()
	if version.Generation != 1 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 1, got %d", version.Generation))
	}

	bundle.DeleteObject(policy) // remove obj by namespacedName from bundle
	version = bundle.GetBundleVersion()
	if version.Generation != 2 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 2, got %d", version.Generation))
	}

	policy.UID = "1234"
	bundle.UpdateObject(policy) // add obj to bundle
	version = bundle.GetBundleVersion()
	if version.Generation != 3 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 3, got %d", version.Generation))
	}

	bundle.DeleteObject(policy) // remove obj by uid from bundle
	version = bundle.GetBundleVersion()
	if version.Generation != 4 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 4, got %d", version.Generation))
	}
}

func TestCompleteComplianceStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", 1, extractObjIDFunc)

	completeComplianceStatusBundle := NewCompleteComplianceStatusBundle("leafHubName", bundle,
		1, extractObjIDFunc)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	completeComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version := completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 0 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 0, got %d", version.Generation))
	}

	// create array *policiesv1.CompliancePerClusterStatus{}
	policy.Status = policiesv1.PolicyStatus{
		ComplianceState: policiesv1.NonCompliant,
		Placement: []*policiesv1.Placement{
			{
				PlacementBinding: "test-policy-placement",
				PlacementRule:    "test-policy-placement",
			},
		},
		Status: []*policiesv1.CompliancePerClusterStatus{
			{
				ClusterName:      "hub1-mc1",
				ClusterNamespace: "hub1-mc1",
				ComplianceState:  policiesv1.Compliant,
			},
			{
				ClusterName:      "hub1-mc2",
				ClusterNamespace: "hub1-mc2",
				ComplianceState:  policiesv1.NonCompliant,
			},
		},
	}

	// increase bundle generation in the case where cluster lists were changed
	completeComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 1 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 1, got %d", version.Generation))
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent).
	completeComplianceStatusBundle.DeleteObject(policy)
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 1 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 1, got %d", version.Generation))
	}

	policy.UID = "1234"
	completeComplianceStatusBundle.DeleteObject(policy)
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 1 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 1, got %d", version.Generation))
	}
}

func TestDeltaComplianceStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", 1, extractObjIDFunc)
	completeComplianceStatusBundle := NewCompleteComplianceStatusBundle("leafHubName", bundle,
		1, extractObjIDFunc)

	deltaComplianceStatusBundle := NewDeltaComplianceStatusBundle("leafHubName", completeComplianceStatusBundle,
		bundle.(*ClustersPerPolicyBundle), 1, extractObjIDFunc)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	runtimePolicy := policy.DeepCopy()
	runtimePolicy.SetUID("1234")
	// policy is new then sync what's in the clustersPerPolicy base
	deltaComplianceStatusBundle.UpdateObject(runtimePolicy) // add obj to bundle
	version := deltaComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 0 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 0, got %d", version.Generation))
	}

	deltaComplianceStatusBundle.DeleteObject(runtimePolicy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 0 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 0, got %d", version.Generation))
	}

	deltaComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 0 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 0, got %d", version.Generation))
	}

	deltaComplianceStatusBundle.DeleteObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	if version.Generation != 0 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 0, got %d", version.Generation))
	}
}

func TestMinimalComplianceStatusBundle(t *testing.T) {
	bundle := NewMinimalComplianceStatusBundle("leafHubName", 1)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	runtimePolicy := policy.DeepCopy()
	runtimePolicy.SetAnnotations(map[string]string{
		constants.OriginOwnerReferenceAnnotation: "1234",
	})
	bundle.UpdateObject(runtimePolicy)
	version := bundle.GetBundleVersion()
	if version.Generation != 1 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 1, got %d", version.Generation))
	}

	bundle.DeleteObject(runtimePolicy)
	version = bundle.GetBundleVersion()
	if version.Generation != 2 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 2, got %d", version.Generation))
	}

	bundle.UpdateObject(runtimePolicy)
	version = bundle.GetBundleVersion()
	if version.Generation != 3 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 3, got %d", version.Generation))
	}
	bundle.DeleteObject(policy)
	version = bundle.GetBundleVersion()
	if version.Generation != 4 {
		t.Fatal(fmt.Errorf("expected version.Generation to be 4, got %d", version.Generation))
	}
}