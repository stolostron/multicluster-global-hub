package grc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	bundle "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestClusterPerPolicyStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", extractObjIDFunc)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	bundle.UpdateObject(policy) // add obj to bundle
	version := bundle.GetBundleVersion()
	assert.Equal(t, "0.1", version.String())

	bundle.UpdateObject(policy) // add obj to bundle
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.1", version.String())

	bundle.DeleteObject(policy) // remove obj by namespacedName from bundle
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.2", version.String())

	policy.UID = "1234"
	bundle.UpdateObject(policy) // add obj to bundle
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.3", version.String())

	bundle.DeleteObject(policy) // remove obj by uid from bundle
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.4", version.String())
}

func TestCompleteComplianceStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", extractObjIDFunc)

	completeComplianceStatusBundle := NewCompleteComplianceStatusBundle("leafHubName", bundle, extractObjIDFunc)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	completeComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version := completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.0", version.String())

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

	// increase bundle version in the case where cluster lists were changed
	completeComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.1", version.String())

	completeComplianceStatusBundle.DeleteObject(policy)
	version = completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.2", version.String())

	policy.UID = "1234"
	completeComplianceStatusBundle.DeleteObject(policy)
	version = completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.2", version.String())
}

func TestDeltaComplianceStatusBundle(t *testing.T) {
	extractObjIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }
	bundle := NewClustersPerPolicyBundle("leafhubname", extractObjIDFunc)
	completeComplianceStatusBundle := NewCompleteComplianceStatusBundle("leafHubName", bundle, extractObjIDFunc)

	deltaComplianceStatusBundle := NewDeltaComplianceStatusBundle("leafHubName", completeComplianceStatusBundle,
		bundle.(*ClustersPerPolicyBundle), extractObjIDFunc)

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
	assert.Equal(t, "0.0", version.String())

	deltaComplianceStatusBundle.UpdateObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.0", version.String())

	deltaComplianceStatusBundle.DeleteObject(policy) // add obj to bundle
	version = completeComplianceStatusBundle.GetBundleVersion()
	assert.Equal(t, "0.0", version.String())
}

func TestMinimalComplianceStatusBundle(t *testing.T) {
	bundle := NewMinimalComplianceStatusBundle("leafHubName")

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
	assert.Equal(t, "0.1", version.String())

	bundle.DeleteObject(runtimePolicy)
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.2", version.String())

	bundle.UpdateObject(runtimePolicy)
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.3", version.String())

	bundle.DeleteObject(policy)
	version = bundle.GetBundleVersion()
	assert.Equal(t, "0.4", version.String())
}
