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

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

func TestAgentClusterRolePhase5RBAC(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	agentDir := filepath.Join(filepath.Dir(thisFile), "..")

	files := []string{
		filepath.Join(agentDir, "manifests", "templates", "agent",
			"multicluster-global-hub-agent-clusterrole.yaml"),
	}

	for _, file := range files {
		t.Run(filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			role := &rbacv1.ClusterRole{}
			if err := yaml.Unmarshal(stripGoTemplateDirectives(raw), role); err != nil {
				t.Fatalf("unmarshal %s: %v", file, err)
			}

			hasClusterRoleCreate := false
			hasClusterRoleBindingCreate := false
			for _, rule := range role.Rules {
				if contains(rule.Verbs, "impersonate") || contains(rule.Verbs, "*") {
					t.Errorf("%s grants impersonate (verbs=%v resources=%v)",
						file, rule.Verbs, rule.Resources)
				}
				if contains(rule.Resources, "*") {
					t.Errorf("%s grants wildcard resources (resources=%v apiGroups=%v)",
						file, rule.Resources, rule.APIGroups)
				}
				if (contains(rule.APIGroups, rbacv1.GroupName) || contains(rule.APIGroups, "*")) &&
					(contains(rule.Resources, "roles") || contains(rule.Resources, "rolebindings")) {
					t.Errorf("%s grants roles/rolebindings for apiGroups=%v (resources=%v)",
						file, rule.APIGroups, rule.Resources)
				}
				if contains(rule.APIGroups, rbacv1.GroupName) && contains(rule.Verbs, "create") {
					if contains(rule.Resources, "clusterroles") {
						hasClusterRoleCreate = true
					}
					if contains(rule.Resources, "clusterrolebindings") {
						hasClusterRoleBindingCreate = true
					}
				}
			}
			if !hasClusterRoleCreate || !hasClusterRoleBindingCreate {
				t.Errorf("%s must keep create on rbac.authorization.k8s.io clusterroles and clusterrolebindings for migration bootstrap", file)
			}
		})
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stripGoTemplateDirectives(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{{") {
			continue
		}
		filtered = append(filtered, line)
	}
	return []byte(strings.Join(filtered, "\n"))
}
