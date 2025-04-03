package addon

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const acmPackageManifestJson = `{
    "apiVersion": "packages.operators.coreos.com/v1",
    "kind": "PackageManifest",
    "metadata": {
        "labels": {
            "catalog": "redhat-operators"
        },
        "name": "advanced-cluster-management",
        "namespace": "multicluster-global-hub"
    },
    "spec": {},
    "status": {
        "catalogSource": "redhat-operators",
        "catalogSourceDisplayName": "Red Hat Operators",
        "catalogSourceNamespace": "openshift-marketplace",
        "catalogSourcePublisher": "Red Hat",
        "channels": [
            {
                "currentCSV": "advanced-cluster-management.v2.6.1",
                "currentCSVDesc": {
                    "apiservicedefinitions": {},
                    "displayName": "Advanced Cluster Management for Kubernetes",
                    "relatedImages": [
                        "registry.redhat.io/rhacm2/acm-prometheus-rhel8@sha256:790b6fe8907b284a4303ad814a8de827d7acc2c6f23c05628b9d9bb22faec1e0",
                        "registry.redhat.io/rhacm2/governance-policy-propagator-rhel8:release-2.6"
                    ],
                    "version": "2.6.1"
                },
                "name": "release-2.6"
            }
        ],
        "defaultChannel": "release-2.6",
        "packageName": "advanced-cluster-management",
        "provider": {
            "name": "Red Hat"
        }
    }
}`

const mcePachageManifestJson = `{
    "apiVersion": "packages.operators.coreos.com/v1",
    "kind": "PackageManifest",
    "metadata": {
        "creationTimestamp": "2022-09-25T16:17:14Z",
        "labels": {
            "catalog": "redhat-operators"
        },
        "name": "multicluster-engine",
        "namespace": "multicluster-global-hub"
    },
    "spec": {},
    "status": {
        "catalogSource": "multiclusterengine-catalog",
        "catalogSourceDisplayName": "MultiCluster Engine",
        "catalogSourceNamespace": "openshift-marketplace",
        "catalogSourcePublisher": "Red Hat",
        "channels": [
            {
                "currentCSV": "multicluster-engine.v2.2.0",
                "currentCSVDesc": {
                    "displayName": "multicluster engine for Kubernetes",
                    "relatedImages": [
                        "quay.io/stolostron/provider-credential-controller@sha256:5812d54a0d808b275f66a4367a0d9a96a24260bc7ce6b1725f5852858cc8875a",
                        "quay.io/stolostron/cluster-proxy@sha256:3ad61eace84304d2d7d7b13b13d6712a53ec24a2a8bc49b3e2e48fbcfb34d0f7"
                    ],
                    "version": "2.2.0"
                },
                "name": "stable-2.2"
            }
        ],
        "defaultChannel": "stable-2.2",
        "packageName": "multicluster-engine",
        "provider": {
            "name": "Red Hat"
        }
    }
}`

func fakeACMPackageManifests() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	_ = obj.UnmarshalJSON([]byte(acmPackageManifestJson))
	return obj
}

func fakeMCEPackageManifests() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	_ = obj.UnmarshalJSON([]byte(mcePachageManifestJson))
	return obj
}

func TestGetPackageManifestConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, fakeACMPackageManifests(), fakeMCEPackageManifests())
	pm, err := GetPackageManifestConfig(context.TODO(), dynamicClient)
	if err != nil {
		t.Errorf("failed to get packageManfiestConfig. err = %v", err)
	}
	if pm.MCEDefaultChannel != "stable-2.2" ||
		pm.MCECurrentCSV != "multicluster-engine.v2.2.0" ||
		pm.ACMDefaultChannel != "release-2.6" ||
		pm.ACMCurrentCSV != "advanced-cluster-management.v2.6.1" {
		t.Errorf("failed to get correct pacakgeManifestConfig. %v", pm)
	}
}
