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
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestSpecEventSourceAllowed(t *testing.T) {
	t.Run("accepts global-hub source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource(constants.CloudEventGlobalHubClusterName)
		if !specEventSourceAllowed(&evt, "hub2") {
			t.Fatal("expected global-hub source to be allowed")
		}
	})

	t.Run("rejects spoofed generic spec source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetType(constants.GenericSpecMsgKey)
		evt.SetSource("victim-hub")
		if specEventSourceAllowed(&evt, "hub2") {
			t.Fatal("expected spoofed source to be rejected")
		}
	})

	t.Run("accepts registered migration deploy source", func(t *testing.T) {
		syncers.RegisterMigrationDeploySources("hub2", "migration-1", []string{"hub1"})
		t.Cleanup(func() { syncers.ClearMigrationDeploySources("hub2", "migration-1") })

		evt := cloudevents.NewEvent()
		evt.SetType(constants.MigrationTargetMsgKey)
		evt.SetSource("hub1")
		if !specEventSourceAllowed(&evt, "hub2") {
			t.Fatal("expected registered migration deploy source to be allowed")
		}
	})

	t.Run("rejects unregistered migration deploy source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetType(constants.MigrationTargetMsgKey)
		evt.SetSource("spoofed-hub")
		if specEventSourceAllowed(&evt, "hub2") {
			t.Fatal("expected unregistered migration deploy source to be rejected")
		}
	})
}
