package enhancers

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var PolicyMessageStatusRe = regexp.MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)

type PolicyEventEnhancer struct {
	runtimeClient client.Client
	log           logr.Logger
}

func NewPolicyEventEnhancer(runtimeClient client.Client) *PolicyEventEnhancer {
	return &PolicyEventEnhancer{
		runtimeClient: runtimeClient,
		log:           ctrl.Log.WithName("policy-event-enhancer"),
	}
}

func (p *PolicyEventEnhancer) Enhance(ctx context.Context, event *kube.EnhancedEvent) {
	// add policy id and policy compliance state
	rootPolicyNamespacedName, ok := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	if !ok {
		return
	}
	policyNameSlice := strings.Split(rootPolicyNamespacedName, ".")
	if len(policyNameSlice) != 2 {
		p.log.Error(errors.New("invalid root policy name"),
			"failed to get root policy name by label",
			"name", rootPolicyNamespacedName, "label",
			constants.PolicyEventRootPolicyNameLabelKey)
		return
	}
	policy := policyv1.Policy{}
	if err := p.runtimeClient.Get(ctx, client.ObjectKey{Name: policyNameSlice[1], Namespace: policyNameSlice[0]},
		&policy); err != nil {
		p.log.Error(err, "failed to get rootPolicy", "name", rootPolicyNamespacedName,
			"label", constants.PolicyEventRootPolicyNameLabelKey)
		return
	}
	event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey] = string(policy.GetUID())

	parsedPolicyStatus := parsePolicyStatus(event.Message)
	if parsedPolicyStatus != "" {
		event.InvolvedObject.Labels[constants.PolicyEventClusterComplianceLabelKey] = parsedPolicyStatus
	} else {
		event.InvolvedObject.Labels[constants.PolicyEventClusterComplianceLabelKey] = string(policy.Status.ComplianceState)
	}

	// add cluster id
	clusterName, ok := event.InvolvedObject.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return
	}
	cluster := clusterv1.ManagedCluster{}
	if err := p.runtimeClient.Get(ctx, client.ObjectKey{Name: clusterName}, &cluster); err != nil {
		p.log.Error(err, "failed to get cluster", "cluster", clusterName)
	}
	clusterId := string(cluster.GetUID())
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			clusterId = claim.Value
			break
		}
	}
	event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey] = clusterId
}

func parsePolicyStatus(eventMessage string) string {
	matches := PolicyMessageStatusRe.FindStringSubmatch(eventMessage)
	if len(matches) != 4 {
		return ""
	}
	return matches[2]
}
