package policies

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policiesV1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/grc"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	policiesStatusSyncLog = "policies-status-sync"
	rootPolicyLabel       = "policy.open-cluster-management.io/root-policy"
)

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, producer transport.Producer) (*generic.HybridSyncManager, error) {
	leafHubName := agentstatusconfig.GetLeafHubName()
	agentConfig := agentstatusconfig.GetAgentConfigMap()
	bundleCollection, hybridSyncManager, err :=
		createBundleCollection(producer, leafHubName, agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to add policies controller to the manager - %w", err)
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
		agentstatusconfig.GetPolicyDuration); err != nil {
		return hybridSyncManager, fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}
	return hybridSyncManager, nil
}

func createBundleCollection(pro transport.Producer, leafHubName string, agentConfig *corev1.ConfigMap) (
	[]*generic.BundleCollectionEntry, *generic.HybridSyncManager, error,
) {
	// clusters per policy (base bundle)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := grc.NewClustersPerPolicyBundle(leafHubName, extractPolicyID)

	// minimal compliance status bundle
	minimalComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.MinimalPolicyComplianceMsgKey)
	minimalComplianceStatusBundle := grc.NewMinimalComplianceStatusBundle(leafHubName)

	fullStatusPredicate := func() bool { return agentConfig.Data["aggregationLevel"] == "full" }
	minimalStatusPredicate := func() bool {
		return agentConfig.Data["aggregationLevel"] == "minimal"
	}

	// apply a hybrid sync manager on the (full aggregation) compliance bundles
	// completeComplianceStatusBundleCollectionEntry, deltaComplianceStatusBundleCollectionEntry,
	hybridSyncManager, err := getHybridSyncManager(pro, leafHubName, fullStatusPredicate,
		clustersPerPolicyBundle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize hybrid sync manager - %w", err)
	}

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyTransportKey, clustersPerPolicyBundle, fullStatusPredicate),
		// hybridSyncManager.GetBundleCollectionEntry(genericbundle.CompleteStateMode),
		// hybridSyncManager.GetBundleCollectionEntry(genericbundle.DeltaStateMode),
		generic.NewBundleCollectionEntry(minimalComplianceStatusTransportKey, minimalComplianceStatusBundle,
			minimalStatusPredicate),
	}, hybridSyncManager, nil
}

// getHybridComplianceBundleCollectionEntries creates a complete/delta compliance bundle collection entries and has
// them managed by a genericHybridSyncManager.
// The collection entries are returned (or nils with an error if any occurred).
func getHybridSyncManager(producer transport.Producer, leafHubName string,
	fullStatusPredicate func() bool, clustersPerPolicyBundle bundle.Bundle,
) (*generic.HybridSyncManager, error) {
	// complete compliance status bundle
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.PolicyCompleteComplianceMsgKey)
	completeComplianceStatusBundle := grc.NewCompleteComplianceStatusBundle(leafHubName, clustersPerPolicyBundle,
		extractPolicyID)

	// delta compliance status bundle
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.PolicyDeltaComplianceMsgKey)
	deltaComplianceStatusBundle := grc.NewDeltaComplianceStatusBundle(leafHubName, completeComplianceStatusBundle,
		clustersPerPolicyBundle.(*grc.ClustersPerPolicyBundle), extractPolicyID)

	completeComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(completeComplianceStatusTransportKey,
		completeComplianceStatusBundle, fullStatusPredicate)
	deltaComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(deltaComplianceStatusTransportKey,
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
