// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Multicluster hub manager webhook", func() {
	Context("Test Placement and placementrule are handled by the global hub manager webhook", Ordered, func() {

		It("Should not add cluster.open-cluster-management.io/experimental-scheduling-disable annotation to placement", func() {
			testPlacement := &clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placement-",
					Namespace:    utils.GetDefaultNamespace(),
				},
				Spec: clusterv1beta1.PlacementSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacement, &client.CreateOptions{}); err != nil {
					return false
				}
				placement := &clusterv1beta1.Placement{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacement), placement); err != nil {
					return false
				}
				return placement.Annotations == nil
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})


		It("Should not set global-hub as scheduler name for the placementrule", func() {
			testPlacementRule := &placementrulesv1.PlacementRule{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-placementrule-",
					Namespace:    utils.GetDefaultNamespace(),
					Labels:       map[string]string{},
				},
				Spec: placementrulesv1.PlacementRuleSpec{},
			}

			Eventually(func() bool {
				if err := c.Create(ctx, testPlacementRule, &client.CreateOptions{}); err != nil {
					return false
				}
				placementrule := &placementrulesv1.PlacementRule{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(testPlacementRule), placementrule); err != nil {
					return false
				}
				return placementrule.Spec.SchedulerName != constants.GlobalHubSchedulerName
			}, 1*time.Second, 5*time.Second).Should(BeTrue())
		})
	})
})
