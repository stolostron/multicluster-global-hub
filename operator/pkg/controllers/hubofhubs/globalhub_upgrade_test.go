/*
Copyright 2024.

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

package hubofhubs_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	hubofhubscontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./operator/pkg/controllers/hubofhubs -v -ginkgo.focus "MulticlusterGlobalHub upgrade"
var _ = Describe("MulticlusterGlobalHub upgrade", Ordered, func() {
	It("should be able to remove the finalizer from managed hubs", func() {
		By("Create managed hub cluster with cleanup finalizer")
		testMangedCluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-managed-hub",
				Labels: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
				Annotations: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
				Finalizers: []string{
					commonconstants.GlobalHubCleanupFinalizer,
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		}
		Expect(k8sClient.Create(ctx, testMangedCluster)).Should(Succeed())

		By("Create the reconciler")
		mghReconciler = &hubofhubscontroller.MulticlusterGlobalHubReconciler{
			Client: k8sClient,
			Log:    ctrl.Log.WithName("multicluster-global-hub-reconciler"),
		}
		err := mghReconciler.Upgrade(ctx)
		Expect(err).Should(Succeed())

		By("Check the finalizer should be removed from the managed hub cluster")
		Eventually(func() error {
			clusters := &clusterv1.ManagedClusterList{}
			if err := k8sClient.List(ctx, clusters, &client.ListOptions{}); err != nil {
				return err
			}

			for idx := range clusters.Items {
				managedHub := &clusters.Items[idx]
				if managedHub.Name == constants.LocalClusterName {
					continue
				}

				ok := controllerutil.RemoveFinalizer(managedHub, commonconstants.GlobalHubCleanupFinalizer)
				if ok {
					return fmt.Errorf("the finalizer should be removed from cluster %s", managedHub.GetName())
				}
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})
