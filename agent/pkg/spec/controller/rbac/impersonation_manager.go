package rbac

import (
	"encoding/base64"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NoIdentity is used to mark no identity is defined on a resource.
	NoIdentity = ""
	// UserIdentityAnnotation is the annotation that is used to store the user identity.
	UserIdentityAnnotation = "open-cluster-management.io/user-identity"
	// UserGroupsAnnotation is the annotation that is used to store the user groups.
	UserGroupsAnnotation = "open-cluster-management.io/user-group"
)

// NewImpersonationManager creates a new instance of ImpersonationManager.
func NewImpersonationManager(config *rest.Config) *ImpersonationManager {
	return &ImpersonationManager{
		k8sConfig: config,
	}
}

// ImpersonationManager manages the k8s clients for the various users and for the controller.
type ImpersonationManager struct {
	k8sConfig *rest.Config
}

// Impersonate gets the user identity and returns the k8s client that represents the requesting user.
func (manager *ImpersonationManager) Impersonate(userIdentity string, userGroups []string) (client.Client, error) {
	newConfig := rest.CopyConfig(manager.k8sConfig)
	newConfig.Impersonate = rest.ImpersonationConfig{
		UserName: userIdentity,
		Groups:   userGroups,
		Extra:    nil,
	}

	userK8sClient, err := client.New(newConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create new k8s client for user - %w", err)
	}

	return userK8sClient, nil
}

// GetUserIdentity returns the user identity in the obj or NoIdentity in case it can't be found on the object.
func (manager *ImpersonationManager) GetUserIdentity(obj interface{}) (string, error) {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return NoIdentity, nil // a custom object
	}

	annotations := unstructuredObj.GetAnnotations()
	if annotations == nil { // there are no annotations defined, therefore user identity is not defined.
		return NoIdentity, nil
	}

	userIdentity, err := manager.decodeBase64IdentityAnnotation(annotations, UserIdentityAnnotation)
	if err != nil {
		return NoIdentity, fmt.Errorf("failed to decode base64 user identity - %w", err)
	}

	return userIdentity, nil
}

// GetUserGroups returns the base64 encoded user groups and the decoded user groups in the obj or nil in case it
// can't be found on the object.
func (manager *ImpersonationManager) GetUserGroups(obj interface{}) (string, []string, error) {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return NoIdentity, nil, nil // a custom object
	}

	annotations := unstructuredObj.GetAnnotations()
	if annotations == nil { // there are no annotations defined, therefore user groups are not defined.
		return NoIdentity, nil, nil
	}

	userGroups, err := manager.decodeBase64IdentityAnnotation(annotations, UserGroupsAnnotation)
	if err != nil {
		return NoIdentity, nil, fmt.Errorf("failed to decode base64 user identity - %w", err)
	}

	return annotations[UserGroupsAnnotation], strings.Split(userGroups, ","), nil // groups is comma separated list
}

func (manager *ImpersonationManager) decodeBase64IdentityAnnotation(annotations map[string]string,
	annotationToDecode string,
) (string, error) {
	if base64Value, found := annotations[annotationToDecode]; found { // if annotation exists
		decodedValue, err := base64.StdEncoding.DecodeString(base64Value)
		if err != nil {
			return NoIdentity, fmt.Errorf("failed to base64 decode annotation %s - %w", annotationToDecode, err)
		}

		return string(decodedValue), nil
	}

	return NoIdentity, nil
}
