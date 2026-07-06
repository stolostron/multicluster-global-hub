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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// BaselineManifestValues is passed to baseline NetworkPolicy templates (allow-dns-and-api, operator NP).
type BaselineManifestValues struct {
	Namespace             string
	PostgresName          string
	APIServerCIDRs        []string
	BYOPostgres           bool
	ExternalPostgresCIDRs []string
	BYOKafka              bool
	ExternalKafkaCIDRs    []string
}

// AgentManifestValues is passed to agent NetworkPolicy templates.
type AgentManifestValues struct {
	Namespace          string
	APIServerCIDRs     []string
	ExternalKafkaCIDRs []string
	WebhookEgressCIDRs []string
}

// BuildBaselineValues resolves API server and BYO Postgres/Kafka CIDRs for hub-namespace baseline policies.
func BuildBaselineValues(ctx context.Context, c client.Client, namespace, postgresName string) BaselineManifestValues {
	values := BaselineManifestValues{
		Namespace:    namespace,
		PostgresName: postgresName,
	}
	cidrs, err := ResolveAPIServerCIDRs(ctx, c)
	if err != nil {
		log.Infow("using ports-only API egress fallback for baseline NetworkPolicy",
			"namespace", namespace, "error", err)
	} else {
		values.APIServerCIDRs = cidrs
	}

	pgSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name: constants.GHStorageSecretName, Namespace: namespace,
	}, pgSecret); err == nil {
		values.BYOPostgres = true
		if uri := string(pgSecret.Data["database_uri"]); uri != "" {
			postgresCIDRs, resolveErr := ResolvePostgresCIDRs(ctx, c, namespace, uri)
			if resolveErr != nil {
				log.Infow("using CNPG/Crunchy pod selector fallback for BYO Postgres egress",
					"namespace", namespace, "error", resolveErr)
			} else {
				values.ExternalPostgresCIDRs = postgresCIDRs
			}
		}
	} else if !apierrors.IsNotFound(err) {
		log.Warnw("failed to read BYO postgres secret for NetworkPolicy egress", "namespace", namespace, "error", err)
	}

	kafkaSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name: constants.GHTransportSecretName, Namespace: namespace,
	}, kafkaSecret); err == nil {
		values.BYOKafka = true
		if bootstrap := string(kafkaSecret.Data["bootstrap_server"]); bootstrap != "" {
			kafkaCIDRs, resolveErr := ResolveBootstrapServerCIDRs(ctx, c, bootstrap, namespace)
			if resolveErr != nil {
				log.Infow("using cross-namespace Strimzi pod selector fallback for BYO Kafka egress",
					"namespace", namespace, "error", resolveErr)
			} else {
				values.ExternalKafkaCIDRs = kafkaCIDRs
			}
		}
	} else if !apierrors.IsNotFound(err) {
		log.Warnw("failed to read BYO kafka secret for NetworkPolicy egress", "namespace", namespace, "error", err)
	}

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
