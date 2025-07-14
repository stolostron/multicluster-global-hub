package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	controllerName      = "leaf-hub-agent-sync"
	notFoundErrorSuffix = "not found"
)

// UpdateObject function updates a given k8s object.
func UpdateObject(ctx context.Context, runtimeClient client.Client, obj *unstructured.Unstructured) error {
	existingObj := obj.DeepCopy()
	err := runtimeClient.Get(ctx, client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, existingObj)
	if err != nil {
		if errors.IsNotFound(err) {
			return runtimeClient.Create(ctx, obj)
		}
		return fmt.Errorf("failed to get object - %w", err)
	}
	obj.SetResourceVersion(existingObj.GetResourceVersion())
	err = runtimeClient.Update(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to update object - %w", err)
	}
	return nil
}

// DeleteObject tries to delete the given object from k8s. returns error and true/false if object was deleted or not.
func DeleteObject(ctx context.Context, k8sClient client.Client, obj *unstructured.Unstructured) (bool, error) {
	if err := k8sClient.Delete(ctx, obj); err != nil {
		if !strings.HasSuffix(err.Error(), notFoundErrorSuffix) {
			return false, fmt.Errorf("failed to delete object - %w", err)
		}
		return false, nil
	}
	return true, nil
}

// get the root policy from the cluster policy label value(namespace.name)
func GetRootPolicy(ctx context.Context, runtimeClient client.Client, namespacedName string) (*policyv1.Policy,
	error,
) {
	// add root policy id
	policyNameSlice := strings.Split(namespacedName, ".")
	if len(policyNameSlice) < 2 {
		return nil, fmt.Errorf("invalid root policy namespaced name - %s", namespacedName)
	}

	rootPolicy := policyv1.Policy{}
	if err := runtimeClient.Get(ctx, client.ObjectKey{
		Namespace: policyNameSlice[0],
		Name:      policyNameSlice[1],
	}, &rootPolicy); err != nil {
		return nil, fmt.Errorf("failed to get root policy - %w", err)
	}
	return &rootPolicy, nil
}

// GetClusterId retrieve the claimId("id.k8s.io") first, if not exist, then get the cluster.UID()
func GetClusterId(ctx context.Context, runtimeClient client.Client, clusterName string) (string, error) {
	cluster := clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, client.ObjectKey{Name: clusterName}, &cluster); err != nil {
		return "", fmt.Errorf("failed to get cluster - %w", err)
	}
	clusterId := string(cluster.GetUID())
	claimID := GetClusterClaimID(&cluster)
	if claimID != "" {
		clusterId = claimID
	}
	return clusterId, nil
}

func GetClusterClaimID(cluster *clusterv1.ManagedCluster) string {
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == constants.ClusterIdClaimName {
			return claim.Value
		}
	}
	return ""
}

func GetObjectKey(obj client.Object) string {
	return obj.GetObjectKind().GroupVersionKind().String()
}

func ToCloudEvent(evtType, source, clusterName string, data interface{}) cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType(evtType)
	e.SetSource(source)
	e.SetExtension(constants.CloudEventExtensionKeyClusterName, clusterName)
	_ = e.SetData(cloudevents.ApplicationJSON, data)
	return e
}

func PrettyPrint(obj interface{}) {
	payload, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Println("marshal object error", err)
	} else {
		fmt.Println(string(payload))
	}
}
