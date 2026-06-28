// Copyright Contributors to the Open Cluster Management project.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func specEventSourceAllowed(
	ctx context.Context,
	c client.Client,
	agentConfig *configs.AgentConfig,
	evt *cloudevents.Event,
	subject string,
) bool {
	if evt == nil || agentConfig == nil {
		return false
	}

	source := evt.Source()
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}

	switch evt.Type() {
	case constants.HubHAResourcesMsgKey:
		return hubHAResourceSourceAllowed(agentConfig, source, subject)
	case constants.HAConfigMsgKey:
		return haConfigSourceAllowed(agentConfig, source, subject)
	default:
		if migration.IsMigrationDeployingEvent(evt) {
			return migration.MigrationSourceAllowed(ctx, c, source, subject)
		}
		return false
	}
}

func hubHAResourceSourceAllowed(agentConfig *configs.AgentConfig, source, subject string) bool {
	if agentConfig.GetHubRole() != constants.GHHubRoleStandby {
		return false
	}
	if subject != agentConfig.LeafHubName {
		return false
	}
	if source == "" || source == constants.CloudEventGlobalHubClusterName {
		return false
	}
	return source != agentConfig.LeafHubName
}

func haConfigSourceAllowed(agentConfig *configs.AgentConfig, source, subject string) bool {
	if subject != agentConfig.LeafHubName {
		return false
	}
	if source == "" || source == subject {
		return false
	}
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}
	standbyHub := agentConfig.GetStandbyHub()
	if standbyHub != "" {
		return source == standbyHub
	}
	return true
}
