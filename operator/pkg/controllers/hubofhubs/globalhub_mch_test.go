package hubofhubs

import (
	"testing"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

func TestDisableComponentsInMCH(t *testing.T) {
	cases := []struct {
		name       string
		components []string
		mch        *mchv1.MultiClusterHub
	}{
		{
			name:       "empty components",
			components: []string{},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{},
			},
		},
		{
			name:       "no need to disable single component",
			components: []string{"grc"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{
							{
								Name:    "grc",
								Enabled: false,
							},
						},
					},
				},
			},
		},
		{
			name:       "no need to disable multiple components",
			components: []string{"grc", "app-lifecycle"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{
							{
								Name:    "grc",
								Enabled: false,
							},
							{
								Name:    "app-lifecycle",
								Enabled: false,
							},
						},
					},
				},
			},
		},
		{
			name:       "disable one component explicitly",
			components: []string{"grc", "app-lifecycle"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{
							{
								Name:    "grc",
								Enabled: true,
							},
							{
								Name:    "app-lifecycle",
								Enabled: false,
							},
						},
					},
				},
			},
		},
		{
			name:       "disable multiple components explicitly",
			components: []string{"grc", "app-lifecycle"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{
							{
								Name:    "grc",
								Enabled: true,
							},
							{
								Name:    "app-lifecycle",
								Enabled: true,
							},
						},
					},
				},
			},
		},
		{
			name:       "disable components by adding component explicitly",
			components: []string{"grc", "app-lifecycle"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{
					Overrides: &mchv1.Overrides{
						Components: []mchv1.ComponentConfig{},
					},
				},
			},
		},
		{
			name:       "disable components by adding overrides explicitly",
			components: []string{"grc", "app-lifecycle"},
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			disableComponentsInMCH(tc.mch, tc.components)
			if len(tc.components) != 0 {
				if tc.mch.Spec.Overrides == nil {
					t.Errorf("empty MCH component overrides")
				}
				foundMap := make(map[string]struct{}, len(tc.components))
				for _, c := range tc.mch.Spec.Overrides.Components {
					if utils.Contains(tc.components, c.Name) {
						foundMap[c.Name] = struct{}{}
						if c.Enabled == true {
							t.Errorf("%s is not disabled in MCH", c.Name)
						}
					}
				}
				for _, c := range tc.components {
					if _, existing := foundMap[c]; !existing {
						t.Errorf("%s is not add to MCH component overrides", c)
					}
				}
			}
		})
	}
}
