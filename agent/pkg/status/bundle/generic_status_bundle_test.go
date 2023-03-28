package bundle

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func TestRemoveObjFromBundle(t *testing.T) {
	bundle := NewGenericStatusBundle("leafhubname", 1, nil)

	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	runtimePolicy := policy.DeepCopy()
	runtimePolicy.UID = "1234"
	bundle.UpdateObject(runtimePolicy) // add obj to bundle
	bundle.DeleteObject(policy)        // remove obj by namespacedName from bundle

	runtimePolicy.UID = "1234"
	bundle.UpdateObject(runtimePolicy) // add obj to bundle
	bundle.DeleteObject(runtimePolicy) // remove obj by uid from bundle
}
