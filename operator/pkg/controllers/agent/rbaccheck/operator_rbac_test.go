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
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

func TestOperatorRBACDoesNotAllowImpersonation(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	operatorDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")

	checkRules := func(t *testing.T, rules []rbacv1.PolicyRule) {
		t.Helper()
		if len(rules) == 0 {
			t.Fatal("expected non-empty RBAC rules")
		}
		for _, rule := range rules {
			// Wildcard verbs also grant impersonation. Reject both forms so
			// the operator cannot acquire impersonation through a broader rule.
			if contains(rule.Verbs, "impersonate") || contains(rule.Verbs, "*") {
				t.Errorf("operator grants impersonation: apiGroups=%v resources=%v verbs=%v",
					rule.APIGroups, rule.Resources, rule.Verbs)
			}
		}
	}

	t.Run("generated ClusterRole", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(filepath.Join(operatorDir, "config", "rbac", "role.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var role rbacv1.ClusterRole
		if err := yaml.Unmarshal(raw, &role); err != nil {
			t.Fatal(err)
		}
		checkRules(t, role.Rules)
	})

	t.Run("bundle CSV", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(filepath.Join(operatorDir, "bundle", "manifests",
			"multicluster-global-hub-operator.clusterserviceversion.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		type permission struct {
			ServiceAccountName string              `json:"serviceAccountName"`
			Rules              []rbacv1.PolicyRule `json:"rules"`
		}
		var csv struct {
			Spec struct {
				Install struct {
					Spec struct {
						ClusterPermissions []permission `json:"clusterPermissions"`
						Permissions        []permission `json:"permissions"`
					} `json:"spec"`
				} `json:"install"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(raw, &csv); err != nil {
			t.Fatal(err)
		}
		foundOperator := false
		for _, grant := range csv.Spec.Install.Spec.ClusterPermissions {
			if grant.ServiceAccountName == "multicluster-global-hub-operator" {
				foundOperator = true
			}
			checkRules(t, grant.Rules)
		}
		if !foundOperator {
			t.Fatal("CSV is missing operator clusterPermissions")
		}
		for _, grant := range csv.Spec.Install.Spec.Permissions {
			checkRules(t, grant.Rules)
		}
	})
}
