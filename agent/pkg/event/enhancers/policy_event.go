package enhancers

import (
	"context"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"k8s.io/apimachinery/pkg/api/errors"
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

func (p *PolicyEventEnhancer) Enhance(ctx context.Context, event *kube.EnhancedEvent) bool {
	// add policy id and policy compliance state
	policyNamespace := event.InvolvedObject.Namespace
	policyName := event.InvolvedObject.Name

	// add compliance to event
	if err := p.addPolicyCompliance(ctx, event); err != nil {
		p.log.Error(err, "failed to add policy compliance", "namespace", policyNamespace, "name", policyName)
		return false
	}

	// cluster policy event, then add root policy id and cluster id
	_, ok := event.InvolvedObject.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	// filter the cluster policy event
	return !ok

	// // add root policy id
	// rootPolicy, err := helper.GetRootPolicy(ctx, p.runtimeClient, rootPolicyNamespacedName)
	// if err != nil {
	// 	p.log.Error(err, "failed to get root policy", "namespacedName", rootPolicyNamespacedName)
	// 	return
	// }
	// event.InvolvedObject.Labels[constants.PolicyEventRootPolicyIdLabelKey] = string(rootPolicy.GetUID())

	// // add cluster id
	// clusterName, ok := event.InvolvedObject.Labels[constants.PolicyEventClusterNameLabelKey]
	// if !ok {
	// 	p.log.Error(errors.New("cluster name not found in cluster policy event"), "failed to get cluster name")
	// 	return
	// }
	// clusterId, err := helper.GetClusterId(ctx, p.runtimeClient, clusterName)
	// if err != nil {
	// 	p.log.Error(err, "failed to get cluster id", "clusterName", clusterName)
	// 	return
	// }
	// event.InvolvedObject.Labels[constants.PolicyEventClusterIdLabelKey] = clusterId
}

func (p *PolicyEventEnhancer) addPolicyCompliance(ctx context.Context, event *kube.EnhancedEvent) error {
	labels := event.InvolvedObject.Labels
	if labels == nil {
		event.InvolvedObject.Labels = make(map[string]string)
	}

	parsedCompliance := parsePolicyStatus(event.Message)
	if parsedCompliance != "" {
		event.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey] = parsedCompliance
		return nil
	}

	compliance := "Unknown"
	policy := policyv1.Policy{}
	if err := p.runtimeClient.Get(ctx, client.ObjectKey{
		Name:      event.InvolvedObject.Name,
		Namespace: event.InvolvedObject.Namespace,
	}, &policy); err != nil && !errors.IsNotFound(err) {
		return err
	}

	if policy.Status.ComplianceState != "" {
		compliance = string(policy.Status.ComplianceState)
	}

	event.InvolvedObject.Labels[constants.PolicyEventComplianceLabelKey] = compliance
	return nil
}

func parsePolicyStatus(eventMessage string) string {
	matches := PolicyMessageStatusRe.FindStringSubmatch(eventMessage)
	if len(matches) != 4 {
		return ""
	}
	return matches[2]
}
