// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var _ = Describe("Policy controller", Ordered, func() {
	It("create the Policy in kubernetes", func() {
		testPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-1",
				Namespace: "default",
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: policyv1.PolicySpec{
				Disabled:        true,
				PolicyTemplates: []*policyv1.PolicyTemplate{},
			},
			Status: policyv1.PolicyStatus{
				ComplianceState: policyv1.Compliant,
			},
		}
		Expect(kubeClient.Create(ctx, testPolicy, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the policy from postgres", func() {
		Eventually(func() error {
			db := database.GetGorm()
			var policies []models.SpecPolicy
			err := db.Find(&policies).Error
			if err != nil {
				return err
			}

			for _, p := range policies {
				policy := &policyv1.Policy{}
				err = json.Unmarshal(p.Payload, policy)
				if err != nil {
					return err
				}
				_, ok := policy.Labels[constants.GlobalHubGlobalResourceLabel]
				if policy.GetName() == "test-policy-1" && ok && policy.Spec.Disabled {
					return nil
				}
			}
			return fmt.Errorf("not find created policy in database")
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("update the policy from postgres", func() {
		updatedPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-1",
				Namespace: "default",
			},
		}
		err := kubeClient.Get(ctx, client.ObjectKeyFromObject(updatedPolicy), updatedPolicy)
		Expect(err).Should(Succeed())

		updatedPolicy.Spec.Disabled = false

		err = kubeClient.Update(ctx, updatedPolicy)
		Expect(err).Should(Succeed())

		Eventually(func() error {
			db := database.GetGorm()
			var policies []models.SpecPolicy
			err := db.Find(&policies).Error
			if err != nil {
				return err
			}

			for _, p := range policies {
				policy := &policyv1.Policy{}
				err = json.Unmarshal(p.Payload, policy)
				if err != nil {
					return err
				}
				_, ok := policy.Labels[constants.GlobalHubGlobalResourceLabel]
				if policy.GetName() == "test-policy-1" && ok && policy.Spec.Disabled == false && p.Deleted == false {
					return nil
				}
			}
			return fmt.Errorf("not find updated policy in database")
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("delete the policy from postgres", func() {
		// update the obj
		updatedPolicy := &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-1",
				Namespace: "default",
			},
		}
		err := kubeClient.Get(ctx, client.ObjectKeyFromObject(updatedPolicy), updatedPolicy)
		Expect(err).Should(Succeed())

		err = kubeClient.Delete(ctx, updatedPolicy, &client.DeleteOptions{})
		Expect(err).Should(Succeed())

		Eventually(func() error {
			db := database.GetGorm()
			var policies []models.SpecPolicy
			err := db.Find(&policies).Error
			if err != nil {
				return err
			}

			for _, p := range policies {
				policy := &policyv1.Policy{}
				err = json.Unmarshal(p.Payload, policy)
				if err != nil {
					return err
				}
				_, ok := policy.Labels[constants.GlobalHubGlobalResourceLabel]
				if policy.GetName() == "test-policy-1" && ok && policy.Spec.Disabled == false && p.Deleted {
					return nil
				}
			}
			return fmt.Errorf("not delete policy in database")
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
