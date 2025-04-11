package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestGetLogLevel(t *testing.T) {
	s := scheme.Scheme

	tests := []struct {
		name          string
		configMap     *corev1.ConfigMap
		expectedLevel string
		expectedError bool
		notFoundError bool
	}{
		{
			name: "config map exists with log level",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHConfigCMName,
					Namespace: commonutils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					logger.LogLevelKey: "debug",
				},
			},
			expectedLevel: "debug",
			expectedError: false,
		},
		{
			name: "config map exists without log level",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHConfigCMName,
					Namespace: commonutils.GetDefaultNamespace(),
				},
				Data: map[string]string{},
			},
			expectedLevel: "",
			expectedError: false,
		},
		{
			name:          "config map not found",
			configMap:     nil,
			expectedLevel: string(logger.Info),
			expectedError: false,
			notFoundError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var objects []runtime.Object
			if test.configMap != nil {
				objects = append(objects, test.configMap)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

			level, err := getLogLevel(fakeClient)

			if test.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !test.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if level != test.expectedLevel {
				t.Errorf("expected log level %s but got %s", test.expectedLevel, level)
			}
		})
	}
}
