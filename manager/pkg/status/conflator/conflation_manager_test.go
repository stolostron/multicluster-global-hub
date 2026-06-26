/*
Copyright Contributors to the Open Cluster Management project.

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

package conflator

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/identity"
)

func newTestConflationManager() *ConflationManager {
	return NewConflationManager(statistics.NewStatistics(&statistics.StatisticsConfig{
		LogInterval: "1m",
	}), nil)
}

func TestConflationManagerInsertRejectsSpoofedSource(t *testing.T) {
	cm := newTestConflationManager()
	eventType := string(enum.ManagedClusterType)
	cm.Register(NewConflationRegistration(
		HubClusterHeartbeatPriority,
		enum.CompleteStateMode,
		eventType,
		func(context.Context, *cloudevents.Event) error { return nil },
	))

	evt := cloudevents.NewEvent()
	evt.SetType(eventType)
	evt.SetSource("victim-hub")
	identity.SetAuthedHub(&evt, "writer-hub")

	cm.Insert(&evt)

	if _, found := cm.conflationUnits["victim-hub"]; found {
		t.Fatal("spoofed event must not create a conflation unit for victim-hub")
	}
	if _, found := cm.conflationUnits["writer-hub"]; found {
		t.Fatal("spoofed event must be dropped before conflation unit creation")
	}
}

func TestConflationManagerInsertUsesAuthedHub(t *testing.T) {
	cm := newTestConflationManager()
	eventType := string(enum.ManagedClusterType)
	cm.Register(NewConflationRegistration(
		HubClusterHeartbeatPriority,
		enum.CompleteStateMode,
		eventType,
		func(context.Context, *cloudevents.Event) error { return nil },
	))

	evt := cloudevents.NewEvent()
	evt.SetType(eventType)
	evt.SetSource("hub-a")
	evt.SetExtension(eventversion.ExtVersion, "0.1")
	identity.SetAuthedHub(&evt, "hub-a")

	cm.Insert(&evt)

	if _, found := cm.conflationUnits["hub-a"]; !found {
		t.Fatal("expected conflation unit for hub-a")
	}
	if evt.Source() != "hub-a" {
		t.Fatalf("event source = %q, want hub-a", evt.Source())
	}
}
