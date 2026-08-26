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

package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestSecretPredWatchesPerHubTransportSecretDelete(t *testing.T) {
	perHub := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretNameForCluster("hub1"),
			Namespace: "multicluster-global-hub",
		},
	}
	if !secretPred.Delete(event.DeleteEvent{Object: perHub}) {
		t.Fatal("secretPred must watch deletion of per-hub BYO transport secrets so agents fall back to the shared secret")
	}

	shared := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: "multicluster-global-hub",
		},
	}
	if !secretPred.Delete(event.DeleteEvent{Object: shared}) {
		t.Fatal("secretPred must watch deletion of the shared BYO transport secret")
	}

	other := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated-secret",
			Namespace: "multicluster-global-hub",
		},
	}
	if secretPred.Delete(event.DeleteEvent{Object: other}) {
		t.Fatal("secretPred must ignore unrelated secret deletions")
	}
}
