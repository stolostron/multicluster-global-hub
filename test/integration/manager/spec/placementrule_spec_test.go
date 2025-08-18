// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package spec

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("placementrules controller", Ordered, func() {
	It("create the placementrule in kubernetes", func() {
		testPlacementrule := &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementrule-1",
				Namespace: utils.GetDefaultNamespace(),
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: placementrulev1.PlacementRuleSpec{
				SchedulerName: constants.GlobalHubSchedulerName,
			},
		}
		Expect(runtimeClient.Create(ctx, testPlacementrule, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the placementrule from postgres", func() {
		Eventually(func() error {
			rows, err := database.GetGorm().Raw("SELECT payload FROM spec.placementrules").Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()
			for rows.Next() {

				placementrule := &placementrulev1.PlacementRule{}
				var payload []byte
				err := rows.Scan(&payload)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(payload, placementrule); err != nil {
					return err
				}
				if placementrule.Name == "test-placementrule-1" &&
					placementrule.Spec.SchedulerName == "" {
					return nil
				}
			}
			return fmt.Errorf("not find placementrule in database")
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
