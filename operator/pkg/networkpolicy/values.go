/*
Copyright 2023 Red Hat, Inc.

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

package networkpolicy

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// BaselineManifestValues is passed to baseline NetworkPolicy templates (allow-dns-and-api, operator NP).
type BaselineManifestValues struct {
	Namespace      string
	PostgresName   string
	APIServerCIDRs []string
}

// AgentManifestValues is passed to agent NetworkPolicy templates.
type AgentManifestValues struct {
	Namespace          string
	APIServerCIDRs     []string
	ExternalKafkaCIDRs []string
	WebhookEgressCIDRs []string
}

// BuildBaselineValues resolves API server CIDRs for hub-namespace baseline policies.
func BuildBaselineValues(ctx context.Context, c client.Client, namespace, postgresName string) BaselineManifestValues {
	values := BaselineManifestValues{
		Namespace:    namespace,
		PostgresName: postgresName,
	}
	cidrs, err := ResolveAPIServerCIDRs(ctx, c)
	if err != nil {
		log.Infow("using ports-only API egress fallback for baseline NetworkPolicy",
			"namespace", namespace, "error", err)
		return values
	}
	values.APIServerCIDRs = cidrs
	return values
}

// BuildAgentValues resolves egress CIDRs for a Global Hub agent NetworkPolicy on the local hub cluster.
func BuildAgentValues(ctx context.Context, c client.Client, namespace, bootstrapServer string) AgentManifestValues {
	values := AgentManifestValues{Namespace: namespace}

	cidrs, err := ResolveAPIServerCIDRs(ctx, c)
	if err != nil {
		log.Infow("using ports-only API egress fallback for agent NetworkPolicy",
			"namespace", namespace, "error", err)
	} else {
		values.APIServerCIDRs = cidrs
	}

	if bootstrapServer != "" {
		kafkaCIDRs, err := ResolveBootstrapServerCIDRs(ctx, c, bootstrapServer, namespace)
		if err != nil {
			log.Infow("using broad external Kafka egress fallback for agent NetworkPolicy",
				"namespace", namespace, "error", err)
		} else {
			values.ExternalKafkaCIDRs = kafkaCIDRs
		}
	}

	serviceNetworks, err := ResolveServiceNetworkCIDRs(ctx, c)
	if err != nil {
		log.Warnw("failed to resolve service network CIDR for webhook egress", "error", err)
	} else {
		values.WebhookEgressCIDRs = serviceNetworks
	}

	return values
}

// BuildAddonAgentValues resolves Kafka bootstrap CIDRs for addon agent manifests rendered on the hub.
// API server and webhook CIDRs are resolved on the managed hub at runtime and remain as fallback rules in the template.
func BuildAddonAgentValues(ctx context.Context, c client.Client, bootstrapServer, hubNamespace string) []string {
	if bootstrapServer == "" {
		return nil
	}
	kafkaCIDRs, err := ResolveBootstrapServerCIDRs(ctx, c, bootstrapServer, hubNamespace)
	if err != nil {
		log.Infow("using broad external Kafka egress fallback for addon agent NetworkPolicy", "error", err)
		return nil
	}
	return kafkaCIDRs
}
