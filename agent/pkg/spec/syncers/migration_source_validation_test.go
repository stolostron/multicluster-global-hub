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

import "testing"

func TestMigrationDeploySourceAllowed(t *testing.T) {
	RegisterMigrationDeploySources("hub2", "migration-1", []string{"hub1", "hub3"})
	t.Cleanup(func() { ClearMigrationDeploySources("hub2", "migration-1") })

	if !MigrationDeploySourceAllowed("hub1", "hub2") {
		t.Fatal("expected hub1 to be allowed for hub2")
	}
	if MigrationDeploySourceAllowed("spoofed", "hub2") {
		t.Fatal("expected spoofed hub to be rejected")
	}

	ClearMigrationDeploySources("hub2", "migration-1")
	if MigrationDeploySourceAllowed("hub1", "hub2") {
		t.Fatal("expected deploy source to be cleared")
	}
}
