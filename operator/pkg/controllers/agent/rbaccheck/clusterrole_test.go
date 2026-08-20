// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
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

package rbaccheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentClusterRolePhase5RBAC(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	agentDir := filepath.Join(filepath.Dir(thisFile), "..")

	files := []string{
		filepath.Join(agentDir, "manifests", "clusterrole.yaml"),
		filepath.Join(agentDir, "addon", "manifests", "templates", "agent",
			"multicluster-global-hub-agent-clusterrole.yaml"),
	}

	for _, file := range files {
		t.Run(filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			content := string(raw)

			if strings.Contains(content, "impersonate") {
				t.Errorf("%s still grants impersonate", file)
			}
			if strings.Contains(content, "\n  - roles\n") || strings.Contains(content, "\n  - rolebindings\n") {
				t.Errorf("%s still grants cluster-wide roles/rolebindings", file)
			}
			if !strings.Contains(content, "clusterroles") || !strings.Contains(content, "clusterrolebindings") {
				t.Errorf("%s must keep clusterroles/clusterrolebindings for migration bootstrap", file)
			}
		})
	}
}
