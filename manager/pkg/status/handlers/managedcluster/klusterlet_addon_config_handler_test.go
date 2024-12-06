// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestHandleKlusterletAddonConfigEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add migrationv1alpha1 to scheme: %v", err)
	}
	if err := addonv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add addonv1 to scheme: %v", err)
	}

	evt := cloudevents.NewEvent()
	evt.SetType(string(enum.KlusterletAddonConfigType))
	evt.SetSource(constants.CloudEventSourceGlobalHub)
	evt.SetExtension(eventversion.ExtVersion, "1.0")
	_ = evt.SetData(cloudevents.ApplicationJSON, []byte(`{
"kind": "KlusterletAddonConfig",
"apiVersion": "agent.open-cluster-management.io/v1",
"metadata": {
	"name": "test",
	"namespace": "test"
},
"spec": {
	"proxyConfig": {},
	"searchCollector": {
		"enabled": false
	},
	"policyController": {
		"enabled": false
	},
	"applicationManager": {
		"enabled": false
	},
	"certPolicyController": {
		"enabled": false
	},
	"iamPolicyController": {
		"enabled": false
	}
}}
`))

	cases := []struct {
		name        string
		initObjects []client.Object
		expected    bool
	}{
		{
			name:        "without migration object",
			initObjects: []client.Object{},
			expected:    false,
		},
		{
			name: "with migration object",
			initObjects: []client.Object{
				&migrationv1alpha1.ManagedClusterMigration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "migration",
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
						IncludedManagedClusters: []string{"test"},
						From:                    "hub1",
						To:                      "hub2",
					},
				},
			},
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(c.initObjects...).WithObjects(c.initObjects...).Build()
			handler := &klusterletAddonConfigHandler{
				client: client,
				log:    logger.ZapLogger("klusterlet-addon-config-handler"),
			}
			handler.handleKlusterletAddonConfigEvent(context.Background(), &evt)
			migration := migrationv1alpha1.ManagedClusterMigration{}
			client.Get(context.Background(), types.NamespacedName{Name: "migration", Namespace: utils.GetDefaultNamespace()}, &migration)
			_, desired := migration.Annotations[constants.KlusterletAddonConfigAnnotation]
			if desired != c.expected {
				t.Error("failed to get the expected result")
			}
		})
	}
}
