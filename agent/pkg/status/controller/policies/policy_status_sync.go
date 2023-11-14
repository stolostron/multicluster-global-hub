package policies

import (
	"errors"
	"fmt"

	policiesV1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	policiesStatusSyncLog = "policies-status-sync"
	rootPolicyLabel       = "policy.open-cluster-management.io/root-policy"
)

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, producer transport.Producer) (*generic.HybridSyncManager, error) {
	leafHubName := config.GetLeafHubName()
	bundleCollection, hybridSyncManager, err := createBundleCollection(producer, leafHubName)
	if err != nil {
		return nil, fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	rootPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasLabel(object, rootPolicyLabel)
	})

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	createObjFunction := func() bundle.Object { return &policiesV1.Policy{} }

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewStatusGenericSyncer(mgr, policiesStatusSyncLog, producer, bundleCollection, createObjFunction,
		predicate.And(rootPolicyPredicate, ownerRefAnnotationPredicate), config.GetPolicyDuration); err != nil {
		return hybridSyncManager, fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}
	return hybridSyncManager, nil
}

func createBundleCollection(pro transport.Producer, leafHubName string) (
	[]*generic.BundleEntry, *generic.HybridSyncManager, error,
) {
	// clusters per policy (base bundle)
	complianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ComplianceMsgKey)
	complianceBundle := grc.NewAgentComplianceBundle(leafHubName, extractPolicyID)

	// minimal compliance status bundle
	minimalComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.MinimalComplianceMsgKey)
	minimalComplianceBundle := grc.NewAgentMinimalComplianceBundle(leafHubName)

	fullStatusPredicate := func() bool { return config.GetAggregationLevel() == config.AggregationFull }
	minimalStatusPredicate := func() bool {
		return config.GetAggregationLevel() == config.AggregationMinimal
	}

	// apply a hybrid sync manager on the (full aggregation) compliance bundles
	// completeComplianceStatusBundleCollectionEntry, deltaComplianceStatusBundleCollectionEntry,
	hybridSyncManager, err := getHybridSyncManager(pro, leafHubName, fullStatusPredicate,
		complianceBundle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize hybrid sync manager - %w", err)
	}

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleEntry{ // multiple bundles for policy status
		generic.NewBundleEntry(complianceTransportKey, complianceBundle, fullStatusPredicate),
		hybridSyncManager.GetBundleCollectionEntry(metadata.CompleteStateMode),
		// hybridSyncManager.GetBundleCollectionEntry(genericbundle.DeltaStateMode),
		generic.NewBundleEntry(minimalComplianceTransportKey, minimalComplianceBundle, minimalStatusPredicate),
	}, hybridSyncManager, nil
}

// getHybridComplianceBundleCollectionEntries creates a complete/delta compliance bundle collection entries and has
// them managed by a genericHybridSyncManager.
// The collection entries are returned (or nils with an error if any occurred).
func getHybridSyncManager(producer transport.Producer, leafHubName string,
	fullStatusPredicate func() bool, complianceBundle bundle.AgentBundle,
) (*generic.HybridSyncManager, error) {
	// complete compliance status bundle
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.CompleteComplianceMsgKey)
	completeComplianceStatusBundle := grc.NewAgentCompleteComplianceBundle(leafHubName, complianceBundle,
		extractPolicyID)

	// delta compliance status bundle
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.DeltaComplianceMsgKey)
	deltaComplianceStatusBundle := grc.NewAgentDeltaComplianceBundle(leafHubName, completeComplianceStatusBundle,
		complianceBundle.(*grc.ComplianceBundle), extractPolicyID)

	completeComplianceBundleCollectionEntry := generic.NewBundleEntry(completeComplianceStatusTransportKey,
		completeComplianceStatusBundle, fullStatusPredicate)
	deltaComplianceBundleCollectionEntry := generic.NewBundleEntry(deltaComplianceStatusTransportKey,
		deltaComplianceStatusBundle, fullStatusPredicate)

	hybridSyncManager, err := generic.NewHybridSyncManager(
		ctrl.Log.WithName("compliance-status-hybrid-sync-manager"),
		completeComplianceBundleCollectionEntry,
		deltaComplianceBundleCollectionEntry)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", err, errors.New(
			"failed to create hybrid sync manager"))
	}
	return hybridSyncManager, nil
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	return val, ok
}
