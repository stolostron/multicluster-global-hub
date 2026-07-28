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

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestSpecEventSourceAllowed(t *testing.T) {
	t.Run("accepts global-hub source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource(constants.CloudEventSourceGlobalHub)
		if !specEventSourceAllowed(&evt) {
			t.Fatal("expected global-hub source to be allowed")
		}
	})

	t.Run("rejects spoofed source", func(t *testing.T) {
		evt := cloudevents.NewEvent()
		evt.SetSource("victim-hub")
		if specEventSourceAllowed(&evt) {
			t.Fatal("expected spoofed source to be rejected")
		}
	})

	t.Run("rejects nil event", func(t *testing.T) {
		if specEventSourceAllowed(nil) {
			t.Fatal("expected nil event to be rejected")
		}
	})
}
