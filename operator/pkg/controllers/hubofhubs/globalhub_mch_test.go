package hubofhubs

import (
	"testing"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

func TestDisableGRCInMCH(t *testing.T) {
	cases := []struct {
		name        string
		mch         *mchv1.MultiClusterHub
		grcDisabled bool
	}{
		{
			name: "no need to disable grc",
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
			grcDisabled: false,
		},
		{
			name: "disable grc explicitly",
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
						},
					},
				},
			},
			grcDisabled: true,
		},
		{
			name: "disable grc by adding component explicitly",
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
			grcDisabled: true,
		},
		{
			name: "disable grc by adding overrides explicitly",
			mch: &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiclusterhub",
					Namespace: config.GetDefaultNamespace(),
				},
				Spec: mchv1.MultiClusterHubSpec{},
			},
			grcDisabled: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newMCH, getGRCDisabled := disableGRCInMCH(tc.mch)
			if newMCH.Spec.Overrides == nil {
				t.Errorf("empty MCH component overrides")
			}
			foundGRC := false
			for _, c := range newMCH.Spec.Overrides.Components {
				if c.Name == "grc" {
					foundGRC = true
					if c.Enabled == true {
						t.Errorf("GRC is not disabled in MCH")
					}
					break
				}
			}
			if !foundGRC {
				t.Errorf("GRC is not add to MCH component overrides")
			}
			if getGRCDisabled != tc.grcDisabled {
				t.Errorf("unexpected grcDisabled, got: %v, want: %v", getGRCDisabled, tc.grcDisabled)
			}
		})
	}
}
