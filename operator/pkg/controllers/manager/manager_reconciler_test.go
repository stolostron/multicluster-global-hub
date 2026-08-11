package manager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestNetworkPolicyPred_Manager(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	managerName := config.COMPONENTS_MANAGER_NAME

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: managerName, Namespace: ns},
	}
	otherNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "other-np", Namespace: ns},
	}

	assert.True(t, networkPolicyPred.Create(event.CreateEvent{Object: np}), "Create: manager NP should match")
	assert.False(t, networkPolicyPred.Create(event.CreateEvent{Object: otherNP}), "Create: other NP should not match")
	assert.True(t, networkPolicyPred.Update(event.UpdateEvent{ObjectNew: np}), "Update: manager NP should match")
	assert.False(t, networkPolicyPred.Update(event.UpdateEvent{ObjectNew: otherNP}), "Update: other NP should not match")
	assert.True(t, networkPolicyPred.Delete(event.DeleteEvent{Object: np}), "Delete: manager NP should match")
	assert.False(t, networkPolicyPred.Delete(event.DeleteEvent{Object: otherNP}), "Delete: other NP should not match")
}
