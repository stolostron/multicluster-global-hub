package utils

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName      = "leaf-hub-agent-sync"
	notFoundErrorSuffix = "not found"
)

// UpdateObject function updates a given k8s object.
func UpdateObject(ctx context.Context, runtimeClient client.Client, obj *unstructured.Unstructured) error {
	objectBytes, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to update object - %w", err)
	}
	forceChanges := true
	if err := runtimeClient.Patch(ctx, obj, client.RawPatch(types.ApplyPatchType, objectBytes), &client.PatchOptions{
		FieldManager: controllerName,
		Force:        &forceChanges,
		Raw: &metav1.PatchOptions{
			FieldValidation: metav1.FieldValidationIgnore,
		},
	}); err != nil {
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

func GetClusterId(ctx context.Context, runtimeClient client.Client, clusterName string) (string, error) {
	cluster := clusterv1.ManagedCluster{}
	if err := runtimeClient.Get(ctx, client.ObjectKey{Name: clusterName}, &cluster); err != nil {
		return "", fmt.Errorf("failed to get cluster - %w", err)
	}
	clusterId := string(cluster.GetUID())
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			clusterId = claim.Value
			break
		}
	}
	return clusterId, nil
}
