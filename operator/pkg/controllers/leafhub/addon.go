/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leafhub

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// applyClusterManagementAddon creates or updates ClusterManagementAddOn for multicluster-global-hub
func applyClusterManagementAddon(ctx context.Context, c client.Client, log logr.Logger) error {
	new := buildClusterManagementAddon()
	existing := &addonv1alpha1.ClusterManagementAddOn{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name: new.GetName(),
		}, existing); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating hoh clustermanagementaddon", "name", new.GetName())
			return c.Create(ctx, new)
		} else {
			// Error reading the object
			return err
		}
	}

	if !equality.Semantic.DeepDerivative(new.Spec, existing.Spec) ||
		!equality.Semantic.DeepDerivative(new.GetLabels(), existing.GetLabels()) ||
		!equality.Semantic.DeepDerivative(new.GetAnnotations(), existing.GetAnnotations()) {
		log.Info("updating hoh clustermanagementaddon because it is changed", "name", new.GetName())
		new.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
		return c.Update(ctx, new)
	}

	log.Info("hoh clustermanagementaddon is existing and not changed")

	return nil
}

// deleteClusterManagementAddon deletes ClusterManagementAddOn for multicluster-global-hub
func deleteClusterManagementAddon(ctx context.Context, c client.Client, log logr.Logger) error {
	clusterManagementAddOn := buildClusterManagementAddon()
	err := c.Delete(ctx, clusterManagementAddOn)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to delete hoh clustermanagementaddon", "name", clusterManagementAddOn.GetName())
		return err
	}

	log.Info("hoh clustermanagementaddon is deleted", "name", clusterManagementAddOn.GetName())
	return nil
}

// buildClusterManagementAddon builds ClusterManagementAddOn resource for multicluster-global-hub
func buildClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.HoHClusterManagementAddonName,
			Labels: map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: constants.HoHClusterManagementAddonDisplayName,
				Description: constants.HoHClusterManagementAddonDescription,
			},
		},
	}
}

// applyManagedClusterAddon creates or updates ManagedClusterAddon for leafhubs
func applyManagedClusterAddon(ctx context.Context, c client.Client, log logr.Logger, managedClusterName,
	hostingClusterName, hostedClusterNamespace string,
) error {
	new := buildManagedClusterAddon(managedClusterName, hostingClusterName, hostedClusterNamespace)
	existing := &addonv1alpha1.ManagedClusterAddOn{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      new.GetName(),
			Namespace: new.GetNamespace(),
		}, existing); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating hoh managedclusteraddon", "namespace",
				new.GetNamespace(),
				"name", new.GetName(), "managedcluster", managedClusterName)
			if err := c.Create(ctx, new); err != nil {
				return err
			}
			// update the status of created managedclusteraddon
			// return updateManagedClusterAddonStatus(ctx, c, log, new)
			return nil
		} else {
			// Error reading the object
			return err
		}
	}

	// compare new managedclusteraddon and existing managedclusteraddon
	if !(equality.Semantic.DeepDerivative(new.Spec, existing.Spec) ||
		!equality.Semantic.DeepDerivative(new.GetLabels(), existing.GetLabels()) ||
		!equality.Semantic.DeepDerivative(new.GetAnnotations(), existing.GetAnnotations())) {
		log.Info("updating hoh managedclusteraddon because it is changed",
			"namespace", new.GetNamespace(),
			"name", new.GetName(),
			"managedcluster", managedClusterName)
		new.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
		return c.Update(ctx, new)
	}

	log.Info("hoh managedclusteraddon is existing and not changed",
		"namespace", new.GetNamespace(),
		"name", new.GetName(), "managedcluster", managedClusterName)

	return nil
}

// deleteManagedClusterAddon deletes ManagedClusterAddon for leafhubs
func deleteManagedClusterAddon(ctx context.Context, c client.Client, log logr.Logger, managedClusterName string,
) error {
	managedClusterAddon := buildManagedClusterAddon(managedClusterName, "", "")
	err := c.Delete(ctx, managedClusterAddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to delete hoh managedclusteraddon", managedClusterAddon.GetNamespace(),
			"name", managedClusterAddon.GetName(), "managedcluster", managedClusterName)
		return err
	}

	log.Info("hoh managedclusteraddon is deleted", "namespace", managedClusterAddon.GetNamespace(),
		"name", managedClusterAddon.GetName(), "managedcluster", managedClusterName)
	return nil
}

// buildManagedClusterAddon builds ManagedClusterAddOn resource for given managedcluster
func buildManagedClusterAddon(managedClusterName, hostingClusterName, hostedClusterNamespace string,
) *addonv1alpha1.ManagedClusterAddOn {
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.HoHManagedClusterAddonName,
			Namespace: managedClusterName,
			Labels: map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			// addon lease namespace, default should be the namespace of multicluster-global-hub-agent
			InstallNamespace: constants.HOHDefaultNamespace,
		},
	}

	if hostingClusterName != "" {
		managedClusterAddon.SetAnnotations(map[string]string{
			constants.AnnotationClusterHostingClusterName: hostingClusterName,
		})
		managedClusterAddon.Spec = addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: hostedClusterNamespace,
		}
	}

	return managedClusterAddon
}

// updateManagedClusterAddonStatus updates the status of given ManagedClusterAddOn
func updateManagedClusterAddonStatus(ctx context.Context, c client.Client, log logr.Logger,
	managedClusterAddon *addonv1alpha1.ManagedClusterAddOn,
) error {
	// wait 10s for readiness of created managedclusteraddon with 2 seconds interval
	existing := &addonv1alpha1.ManagedClusterAddOn{}
	if errPoll := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
		if err := c.Get(ctx,
			types.NamespacedName{
				Name:      managedClusterAddon.GetName(),
				Namespace: managedClusterAddon.GetNamespace(),
			}, existing); err != nil {
			return false, err
		}
		return true, nil
	}); errPoll != nil {
		log.Error(errPoll, "failed to get the managedclusteraddon",
			"namespace", managedClusterAddon.GetNamespace(), "name", managedClusterAddon.GetName())
		return errPoll
	}

	// got the created managedclusteraddon just now, updating its status
	existing.Status.AddOnMeta = addonv1alpha1.AddOnMeta{
		DisplayName: constants.HoHManagedClusterAddonDisplayName,
		Description: constants.HoHManagedClusterAddonDescription,
	}

	newCondition := metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "ManifestWorkCreated",
		Message:            "Addon Installing",
	}
	if len(existing.Status.Conditions) > 0 {
		existing.Status.Conditions = append(existing.Status.Conditions, newCondition)
	} else {
		existing.Status.Conditions = []metav1.Condition{newCondition}
	}

	// update status for the created managedclusteraddon
	if err := c.Status().Update(context.TODO(), existing); err != nil {
		log.Error(err, "failed to update status for managedclusteraddon",
			"namespace", existing.GetNamespace(), "name", existing.GetName())
		return err
	}

	log.Info("updated the status of managedclusteraddon",
		"namespace", existing.GetNamespace(), "name", existing.GetName())

	return nil
}
