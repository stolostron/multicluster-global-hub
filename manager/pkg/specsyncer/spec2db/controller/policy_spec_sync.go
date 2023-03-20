// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/governance-policy-propagator/controllers/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddPolicyController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&policyv1.Policy{}).
		WithEventFilter(GlobalResourcePredicate()).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("policies-spec-syncer"),
			tableName:      "policies",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &policyv1.Policy{} },
			cleanObject:    cleanPolicyStatus,
			areEqual:       arePoliciesEqual,
		}); err != nil {
		return fmt.Errorf("failed to add policy controller to the manager: %w", err)
	}

	return nil
}

func cleanPolicyStatus(instance client.Object) {
	policy, ok := instance.(*policyv1.Policy)

	if !ok {
		panic("wrong instance passed to cleanPolicyStatus: not a Policy")
	}

	policy.Status = policyv1.PolicyStatus{}
}

func arePoliciesEqual(instance1, instance2 client.Object) bool {
	policy1, ok1 := instance1.(*policyv1.Policy)
	policy2, ok2 := instance2.(*policyv1.Policy)

	if !ok1 || !ok2 {
		return false
	}

	// TODO handle Template comparison later
	policy1WithoutTemplates := policy1.DeepCopy()
	policy1WithoutTemplates.Spec.PolicyTemplates = nil

	policy2WithoutTemplates := policy2.DeepCopy()
	policy2WithoutTemplates.Spec.PolicyTemplates = nil

	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return common.CompareSpecAndAnnotation(policy1WithoutTemplates, policy2WithoutTemplates) && labelsMatch
}
