package security

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientpkg "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	mutex            sync.Mutex
	stackroxCentrals map[stackroxCentral]*stackroxCentralData = map[stackroxCentral]*stackroxCentralData{}
	centralCRGVK     schema.GroupVersionKind                  = schema.GroupVersionKind{
		Group:   "platform.stackrox.io",
		Version: "v1alpha1",
		Kind:    "Central",
	}
)

type stackroxCentral struct {
	name      string
	namespace string
}

type stackroxCentralData struct {
	externalBaseURL string
	internalBaseURL string
	apiToken        string
}

func getResource(
	ctx context.Context,
	client clientpkg.Client,
	resourceName,
	resourceNamespace string,
	resource clientpkg.Object,
	log logr.Logger,
) (clientpkg.Object, error) {
	resourceKey := types.NamespacedName{
		Name:      resourceName,
		Namespace: resourceNamespace,
	}

	err := client.Get(ctx, resourceKey, resource)
	if apierrors.IsNotFound(err) {
		log.Info("resource was not found", "name", resourceName, "namespace", resourceNamespace)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get resource (name: %s, namespace: %s): %v", resourceName, resourceNamespace, err)
	}
	log.Info("found resource", "name", resourceName, "namespace", resourceNamespace, "kind", resource.GetObjectKind())

	return resource, nil
}
