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
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func specEventSourceAllowed(evt *cloudevents.Event, leafHubName string) bool {
	if evt == nil {
		return false
	}

	source := evt.Source()
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}

	if evt.Type() == constants.MigrationTargetMsgKey {
		return syncers.MigrationDeploySourceAllowed(source, leafHubName)
	}

	return false
}
