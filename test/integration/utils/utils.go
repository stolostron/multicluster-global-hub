/*
Copyright 2023

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

package utils

import (
	"context"
	"fmt"
	"time"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

func DeleteMgh(ctx context.Context, runtimeClient client.Client, mgh *v1alpha4.MulticlusterGlobalHub) error {
	curmgh := &v1alpha4.MulticlusterGlobalHub{}

	err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), curmgh)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(curmgh.Finalizers) != 0 {
		deletemgh := curmgh.DeepCopy()
		deletemgh.Finalizers = []string{}
		err = runtimeClient.Update(ctx, deletemgh)
		if err != nil {
			return err
		}
	}

	err = runtimeClient.Delete(ctx, mgh)
	if err != nil {
		return err
	}

	if err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh); errors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf("mgh instance should be deleted: %s/%s", mgh.Namespace, mgh.Name)
}

func CreateTransportCSV(c client.Client, ctx context.Context, subName, subNamespace string) error {
	sub := &subv1alpha1.Subscription{}
	err := c.Get(ctx, types.NamespacedName{Name: subName, Namespace: subNamespace}, sub)
	if err != nil {
		if errors.IsNotFound(err) {
			sub = &subv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      subName,
					Namespace: subNamespace,
				},
				Spec: &subv1alpha1.SubscriptionSpec{
					Channel:                "channel-1",
					InstallPlanApproval:    subv1alpha1.ApprovalAutomatic,
					Package:                "packagename-1",
					CatalogSource:          "catalog-1",
					CatalogSourceNamespace: subNamespace,
				},
			}
			err = c.Create(ctx, sub)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	csvName := "csv-1.0"
	sub.Status = subv1alpha1.SubscriptionStatus{
		InstalledCSV: csvName,
		LastUpdated:  metav1.Time{Time: time.Now()},
	}
	err = c.Status().Update(ctx, sub)
	if err != nil {
		return err
	}
	csv := &subv1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      csvName,
			Namespace: subNamespace,
		},
		Spec: subv1alpha1.ClusterServiceVersionSpec{
			InstallStrategy: subv1alpha1.NamedInstallStrategy{
				StrategyName: "deployment",
				StrategySpec: subv1alpha1.StrategyDetailsDeployment{
					DeploymentSpecs: []subv1alpha1.StrategyDeploymentSpec{},
				},
			},
			DisplayName: "strimzi",
		},
	}

	err = c.Create(ctx, csv)
	if err != nil {
		return err
	}
	csv.Status = subv1alpha1.ClusterServiceVersionStatus{
		Phase: subv1alpha1.CSVPhaseSucceeded,
	}
	err = c.Status().Update(ctx, csv)
	return err
}
