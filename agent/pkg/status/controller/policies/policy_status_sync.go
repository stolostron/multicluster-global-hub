package policies

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policiesV1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-globalhub/agent/pkg/helper"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/bundle/grc"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/multicluster-globalhub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-globalhub/pkg/constants"
)

const (
	policiesStatusSyncLog = "policies-status-sync"
	rootPolicyLabel       = "policy.open-cluster-management.io/root-policy"
)

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, producer producer.Producer, env helper.ConfigManager,
	incarnation uint64, hubOfHubsConfig *corev1.ConfigMap, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	bundleCollection, err := createBundleCollection(producer, env, incarnation, hubOfHubsConfig)
	if err != nil {
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	rootPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helper.HasLabel(object, rootPolicyLabel)
	})

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	createObjFunction := func() bundle.Object { return &policiesV1.Policy{} }

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, producer, bundleCollection,
		createObjFunction, predicate.And(rootPolicyPredicate, ownerRefAnnotationPredicate),
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}
	return nil
}

func createBundleCollection(pro producer.Producer, env helper.ConfigManager, incarnation uint64,
	hubOfHubsConfig *corev1.ConfigMap,
) ([]*generic.BundleCollectionEntry, error) {
	deltaSentCountSwitchFactor := env.StatusDeltaCountSwitchFactor
	leafHubName := env.LeafHubName

	// clusters per policy (base bundle)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := grc.NewClustersPerPolicyBundle(leafHubName, incarnation, extractPolicyID)

	// minimal compliance status bundle
	minimalComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.MinimalPolicyComplianceMsgKey)
	minimalComplianceStatusBundle := grc.NewMinimalComplianceStatusBundle(leafHubName, incarnation)

	fullStatusPredicate := func() bool { return hubOfHubsConfig.Data["aggregationLevel"] == "full" }
	minimalStatusPredicate := func() bool {
		return hubOfHubsConfig.Data["aggregationLevel"] == "minimal"
	}

	// apply a hybrid sync manager on the (full aggregation) compliance bundles
	completeComplianceStatusBundleCollectionEntry, deltaComplianceStatusBundleCollectionEntry,
		err := getHybridComplianceBundleCollectionEntries(pro, leafHubName, incarnation, fullStatusPredicate,
		clustersPerPolicyBundle, deltaSentCountSwitchFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize hybrid sync manager - %w", err)
	}

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyTransportKey, clustersPerPolicyBundle, fullStatusPredicate),
		completeComplianceStatusBundleCollectionEntry,
		deltaComplianceStatusBundleCollectionEntry,
		generic.NewBundleCollectionEntry(minimalComplianceStatusTransportKey, minimalComplianceStatusBundle,
			minimalStatusPredicate),
	}, nil
}

// getHybridComplianceBundleCollectionEntries creates a complete/delta compliance bundle collection entries and has
// them managed by a genericHybridSyncManager.
// The collection entries are returned (or nils with an error if any occurred).
func getHybridComplianceBundleCollectionEntries(transport producer.Producer, leafHubName string,
	incarnation uint64, fullStatusPredicate func() bool, clustersPerPolicyBundle bundle.Bundle,
	deltaCountSwitchFactor int,
) (*generic.BundleCollectionEntry, *generic.BundleCollectionEntry, error) {
	// complete compliance status bundle
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.PolicyCompleteComplianceMsgKey)
	completeComplianceStatusBundle := grc.NewCompleteComplianceStatusBundle(leafHubName, clustersPerPolicyBundle,
		incarnation, extractPolicyID)

	// delta compliance status bundle
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.PolicyDeltaComplianceMsgKey)
	deltaComplianceStatusBundle := grc.NewDeltaComplianceStatusBundle(leafHubName, completeComplianceStatusBundle,
		clustersPerPolicyBundle.(*grc.ClustersPerPolicyBundle), incarnation, extractPolicyID)

	completeComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(completeComplianceStatusTransportKey,
		completeComplianceStatusBundle, fullStatusPredicate)
	deltaComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(deltaComplianceStatusTransportKey,
		deltaComplianceStatusBundle, fullStatusPredicate)

	if err := generic.NewHybridSyncManager(ctrl.Log.WithName(
		"compliance-status-hybrid-sync-manager"),
		transport, completeComplianceBundleCollectionEntry, deltaComplianceBundleCollectionEntry,
		deltaCountSwitchFactor); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", err,
			errors.New("failed to create hybrid sync manager"))
	}

	return completeComplianceBundleCollectionEntry, deltaComplianceBundleCollectionEntry, nil
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	return val, ok
}
