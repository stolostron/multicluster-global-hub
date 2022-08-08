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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs/operator/pkg/constants"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

// applyClusterManagementAddon creates or updates ClusterManagementAddOn for hub-of-hubs
func applyClusterManagementAddon(ctx context.Context, c client.Client, log logr.Logger, hohConfigName string) error {
	newHoHClusterManagementAddOn := buildClusterManagementAddon(hohConfigName)
	existingHoHClusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name: newHoHClusterManagementAddOn.GetName(),
		}, existingHoHClusterManagementAddOn); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating hoh clustermanagementaddon", "name", newHoHClusterManagementAddOn.GetName())
			return c.Create(ctx, newHoHClusterManagementAddOn)
		} else {
			// Error reading the object
			return err
		}
	}

	if !equality.Semantic.DeepDerivative(newHoHClusterManagementAddOn.Spec, existingHoHClusterManagementAddOn.Spec) {
		log.Info("updating hoh clustermanagementaddon because it is changed", "name", newHoHClusterManagementAddOn.GetName())
		newHoHClusterManagementAddOn.ObjectMeta.ResourceVersion = existingHoHClusterManagementAddOn.ObjectMeta.ResourceVersion
		return c.Update(ctx, newHoHClusterManagementAddOn)
	}

	log.Info("hoh clustermanagementaddon is existing and not changed")

	return nil
}

// deleteClusterManagementAddon deletes ClusterManagementAddOn for hub-of-hubs
func deleteClusterManagementAddon(ctx context.Context, c client.Client, log logr.Logger, hohConfigName string) error {
	hohClusterManagementAddOn := buildClusterManagementAddon(hohConfigName)
	err := c.Delete(ctx, hohClusterManagementAddOn)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to delete hoh clustermanagementaddon", "name", hohClusterManagementAddOn.GetName())
		return err
	}

	log.Info("hoh clustermanagementaddon is deleted", "name", hohClusterManagementAddOn.GetName())
	return nil
}

// buildClusterManagementAddon builds ClusterManagementAddOn resource for hub-of-hubs
func buildClusterManagementAddon(hohConfigName string) *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.HoHClustermanagementAddonName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: hohConfigName,
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: constants.HoHClustermanagementAddonDisplayName,
				Description: constants.HoHClustermanagementAddonDescription,
			},
		},
	}
}

// applyManagedClusterAddon creates or updates ManagedClusterAddon for leafhubs
func applyManagedClusterAddon(ctx context.Context, c client.Client, log logr.Logger, managedClusterName, hohConfigName string) error {
	newHoHManagedClusterAddon := buildManagedClusterAddon(managedClusterName, hohConfigName)
	existingHoHManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      newHoHManagedClusterAddon.GetName(),
			Namespace: newHoHManagedClusterAddon.GetNamespace(),
		}, existingHoHManagedClusterAddon); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating hoh managedclusteraddon", "namespace", newHoHManagedClusterAddon.GetNamespace(), "name", newHoHManagedClusterAddon.GetName(), "managedcluster", managedClusterName)
			if err := c.Create(ctx, newHoHManagedClusterAddon); err != nil {
				return err
			}
			// update the status of created managedclusteraddon
			// return updateManagedClusterAddonStatus(ctx, c, log, newHoHManagedClusterAddon)
			return nil
		} else {
			// Error reading the object
			return err
		}
	}

	// compare new managedclusteraddon and existing managedclusteraddon
	if !equality.Semantic.DeepDerivative(newHoHManagedClusterAddon.Spec, existingHoHManagedClusterAddon.Spec) {
		log.Info("updating hoh managedclusteraddon because it is changed", "namespace", newHoHManagedClusterAddon.GetNamespace(), "name", newHoHManagedClusterAddon.GetName(), "managedcluster", managedClusterName)
		newHoHManagedClusterAddon.ObjectMeta.ResourceVersion = existingHoHManagedClusterAddon.ObjectMeta.ResourceVersion
		return c.Update(ctx, newHoHManagedClusterAddon)
	}

	log.Info("hoh managedclusteraddon is existing and not changed", "namespace", newHoHManagedClusterAddon.GetNamespace(), "name", newHoHManagedClusterAddon.GetName(), "managedcluster", managedClusterName)

	return nil
}

// deleteManagedClusterAddon deletes ManagedClusterAddon for leafhubs
func deleteManagedClusterAddon(ctx context.Context, c client.Client, log logr.Logger, managedClusterName, hohConfigName string) error {
	hohManagedClusterAddon := buildManagedClusterAddon(managedClusterName, hohConfigName)
	err := c.Delete(ctx, hohManagedClusterAddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to delete hoh managedclusteraddon", hohManagedClusterAddon.GetNamespace(), "name", hohManagedClusterAddon.GetName(), "managedcluster", managedClusterName)
		return err
	}

	log.Info("hoh managedclusteraddon is deleted", "namespace", hohManagedClusterAddon.GetNamespace(), "name", hohManagedClusterAddon.GetName(), "managedcluster", managedClusterName)
	return nil
}

// buildManagedClusterAddon builds ManagedClusterAddOn resource for given managedcluster
func buildManagedClusterAddon(managedClusterName, hohConfigName string) *addonv1alpha1.ManagedClusterAddOn {
	return &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.HoHManagedClusterAddonName,
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: hohConfigName,
			},
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			// addon lease namespace, should be the namespace of hub-of-hubs-agent
			InstallNamespace: constants.HoHManagedClusterAddonInstallationNamespace,
		},
	}
}

// updateManagedClusterAddonStatus updates the status of given ManagedClusterAddOn
func updateManagedClusterAddonStatus(ctx context.Context, c client.Client, log logr.Logger, managedClusterAddon *addonv1alpha1.ManagedClusterAddOn) error {
	// wait 10s for readiness of created managedclusteraddon with 2 seconds interval
	existingHoHManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	if errPoll := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
		if err := c.Get(ctx,
			types.NamespacedName{
				Name:      managedClusterAddon.GetName(),
				Namespace: managedClusterAddon.GetNamespace(),
			}, existingHoHManagedClusterAddon); err != nil {
			return false, err
		}
		return true, nil
	}); errPoll != nil {
		log.Error(errPoll, "failed to get the managedclusteraddon", "namespace", managedClusterAddon.GetNamespace(), "name", managedClusterAddon.GetName())
		return errPoll
	}

	// got the created managedclusteraddon just now, updating its status
	existingHoHManagedClusterAddon.Status.AddOnMeta = addonv1alpha1.AddOnMeta{
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
	if len(existingHoHManagedClusterAddon.Status.Conditions) > 0 {
		existingHoHManagedClusterAddon.Status.Conditions = append(existingHoHManagedClusterAddon.Status.Conditions, newCondition)
	} else {
		existingHoHManagedClusterAddon.Status.Conditions = []metav1.Condition{newCondition}
	}

	// update status for the created managedclusteraddon
	if err := c.Status().Update(context.TODO(), existingHoHManagedClusterAddon); err != nil {
		log.Error(err, "failed to update status for managedclusteraddon", "namespace", existingHoHManagedClusterAddon.GetNamespace(), "name", existingHoHManagedClusterAddon.GetName())
		return err
	}

	log.Info("updated the status of managedclusteraddon", "namespace", existingHoHManagedClusterAddon.GetNamespace(), "name", existingHoHManagedClusterAddon.GetName())

	return nil
}
