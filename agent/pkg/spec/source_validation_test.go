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
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestSpecEventSourceAllowed(t *testing.T) {
	ctx := context.Background()

	t.Run("allows global hub events", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource(constants.CloudEventGlobalHubClusterName)
		evt.SetType(constants.GenericSpecMsgKey)
		assert.True(t, specEventSourceAllowed(ctx, nil, "hub2", &evt))
	})

	t.Run("rejects unknown source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("evil")
		evt.SetType(constants.GenericSpecMsgKey)
		assert.False(t, specEventSourceAllowed(ctx, nil, "hub2", &evt))
	})

	t.Run("allows registered migration deploying", func(t *testing.T) {
		t.Cleanup(func() {
			_ = migration.DeleteLocalMigrationCR(ctx, nil, "msa")
		})
		assert.NoError(t, migration.EnsureLocalMigrationCR(ctx, nil, "hub2", &migrationbundle.MigrationTargetBundle{
			FromHub:                   "hub1",
			ManagedClusters:           []string{"c1"},
			ManagedServiceAccountName: "msa",
		}, migrationv1alpha1.PhaseDeploying))

		evt := cloudevents.NewEvent()
		evt.SetSource("hub1")
		evt.SetType(constants.MigrationTargetMsgKey)
		assert.True(t, specEventSourceAllowed(ctx, nil, "hub2", &evt))
	})
}
