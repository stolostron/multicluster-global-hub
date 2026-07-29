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

package syncers

import "sync"

type migrationDeployRecord struct {
	migrationID string
	sourceHubs  map[string]struct{}
}

var (
	migrationDeployMu       sync.RWMutex
	migrationDeployByTarget = make(map[string]migrationDeployRecord)
)

// RegisterMigrationDeploySources records authorized source hubs for migration deploying traffic.
func RegisterMigrationDeploySources(targetHub, migrationID string, sourceHubs []string) {
	if targetHub == "" || migrationID == "" || len(sourceHubs) == 0 {
		return
	}

	hubs := make(map[string]struct{}, len(sourceHubs))
	for _, hub := range sourceHubs {
		if hub == "" {
			continue
		}
		hubs[hub] = struct{}{}
	}
	if len(hubs) == 0 {
		return
	}

	migrationDeployMu.Lock()
	defer migrationDeployMu.Unlock()
	migrationDeployByTarget[targetHub] = migrationDeployRecord{
		migrationID: migrationID,
		sourceHubs:  hubs,
	}
}

// ClearMigrationDeploySources removes in-flight migration deploy authorization for a target hub.
func ClearMigrationDeploySources(targetHub, migrationID string) {
	if targetHub == "" {
		return
	}

	migrationDeployMu.Lock()
	defer migrationDeployMu.Unlock()

	record, ok := migrationDeployByTarget[targetHub]
	if !ok {
		return
	}
	if migrationID != "" && record.migrationID != migrationID {
		return
	}
	delete(migrationDeployByTarget, targetHub)
}

// MigrationDeploySourceAllowed returns true when source may publish deploying bundles to targetHub.
func MigrationDeploySourceAllowed(source, targetHub string) bool {
	if source == "" || targetHub == "" {
		return false
	}

	migrationDeployMu.RLock()
	defer migrationDeployMu.RUnlock()

	record, ok := migrationDeployByTarget[targetHub]
	if !ok {
		return false
	}
	_, ok = record.sourceHubs[source]
	return ok
}
