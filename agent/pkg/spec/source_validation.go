// Copyright (c) 2026 Red Hat, Inc.
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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func specEventSourceAllowed(
	ctx context.Context,
	c client.Client,
	leafHubName string,
	evt *cloudevents.Event,
) bool {
	if evt == nil {
		return false
	}

	source := evt.Source()
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}

	if migration.IsMigrationDeployingEvent(evt) {
		return migration.MigrationSourceAllowed(ctx, c, source, leafHubName)
	}

	return false
}
