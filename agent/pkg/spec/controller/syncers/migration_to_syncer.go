package syncers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
)

type managedClusterMigrationToSyncer struct {
	log     logr.Logger
	client  client.Client
	context context.Context
}

func NewManagedClusterMigrationToSyncer(context context.Context, client client.Client) *managedClusterMigrationToSyncer {
	return &managedClusterMigrationToSyncer{
		log:     ctrl.Log.WithName("managed-cluster-migration-to-syncer"),
		client:  client,
		context: context,
	}
}

func (syncer *managedClusterMigrationToSyncer) Sync(payload []byte) error {
	// handle migration.to cloud event
	managedClusterMigrationToEvent := &bundleevent.ManagedClusterMigrationToEvent{}
	if err := json.Unmarshal(payload, managedClusterMigrationToEvent); err != nil {
		return err
	}

	msaName := managedClusterMigrationToEvent.ManagedServiceAccountName
	msaNamespace := managedClusterMigrationToEvent.ManagedServiceAccountInstallNamespace

	foundClusterManager := &operatorv1.ClusterManager{}
	namespacedName := types.NamespacedName{Name: "cluster-manager"}
	if err := syncer.client.Get(syncer.context, namespacedName, foundClusterManager); err != nil {
		return err
	}

	clusterManager := foundClusterManager.DeepCopy()

	// check if the ManagedClusterAutoApproval feature is enabled and the service account is added to the auto-approve list
	autoApproveUser := fmt.Sprintf("system:serviceaccount:%s:%s", msaNamespace, msaName)
	autoApproveFeatureEnabled := false
	autoApproveUserAdded := false
	clusterManagerChanged := false
	if clusterManager.Spec.RegistrationConfiguration != nil {
		for _, featureGate := range clusterManager.Spec.RegistrationConfiguration.FeatureGates {
			if featureGate.Feature == "ManagedClusterAutoApproval" {
				autoApproveFeatureEnabled = true
				if featureGate.Mode == operatorv1.FeatureGateModeTypeEnable {
					break
				} else {
					featureGate.Mode = operatorv1.FeatureGateModeTypeEnable
					clusterManagerChanged = true
					break
				}
			}
		}
		if !autoApproveFeatureEnabled {
			clusterManager.Spec.RegistrationConfiguration.FeatureGates = append(clusterManager.Spec.RegistrationConfiguration.FeatureGates, operatorv1.FeatureGate{
				Feature: "ManagedClusterAutoApproval",
				Mode:    operatorv1.FeatureGateModeTypeEnable,
			})
			clusterManagerChanged = true
		}
		for _, user := range clusterManager.Spec.RegistrationConfiguration.AutoApproveUsers {
			if user == autoApproveUser {
				autoApproveUserAdded = true
				break
			}
		}
		if !autoApproveUserAdded {
			clusterManager.Spec.RegistrationConfiguration.AutoApproveUsers = append(clusterManager.Spec.RegistrationConfiguration.AutoApproveUsers, autoApproveUser)
			clusterManagerChanged = true
		}
	} else {
		clusterManager.Spec.RegistrationConfiguration = &operatorv1.RegistrationHubConfiguration{
			FeatureGates: []operatorv1.FeatureGate{
				{
					Feature: "ManagedClusterAutoApproval",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
			AutoApproveUsers: []string{
				autoApproveUser,
			},
		}
		clusterManagerChanged = true
	}

	// patch cluster-manager only if it has changed
	if clusterManagerChanged {
		if err := syncer.client.Update(syncer.context, clusterManager); err != nil {
			return err
		}
	}

	// create clusterrolebinding for the service account
	clusterroleBytes := []byte(fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: multicluster-global-hub-migration:%s
rules:
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
`, msaName))

	clusterroleObj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(clusterroleBytes, clusterroleObj); err != nil {
		return err
	}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name: "multicluster-global-hub-migration:" + msaName,
		}, &rbacv1.ClusterRole{}); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating clusterrole", "clusterrole", "multicluster-global-hub-migration:"+msaName)
			if err := syncer.client.Create(syncer.context, clusterroleObj); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	clusterRoleBindingBytes := []byte(fmt.Sprintf(`
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: %s-clusterrolebinding
subjects:
- kind: ServiceAccount
  name:  %s
  namespace: %s
roleRef:
  kind: ClusterRole
  name: system:open-cluster-management:managedcluster:bootstrap:agent-registration
  apiGroup: rbac.authorization.k8s.io
`, msaName, msaName, msaNamespace))

	clusterRoleBindingObj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(clusterRoleBindingBytes, clusterRoleBindingObj); err != nil {
		return err
	}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name: msaName + "-clusterrolebinding",
		}, &rbacv1.ClusterRole{}); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating clusterrolebing", "clusterrolebing", msaName+"-clusterrolebinding")
			if err := syncer.client.Create(syncer.context, clusterRoleBindingObj); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	sarClusterRolebBindingBytes := []byte(fmt.Sprintf(`
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: %s-subjectaccessreviews-clusterrolebinding
subjects:
- kind: ServiceAccount
  name:  %s
  namespace: %s
roleRef:
  kind: ClusterRole
  name: multicluster-global-hub-migration:%s
  apiGroup: rbac.authorization.k8s.io
`, msaName, msaName, msaNamespace, msaName))

	sarClusterRoleBindingObj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(sarClusterRolebBindingBytes, sarClusterRoleBindingObj); err != nil {
		return err
	}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name: msaName + "-subjectaccessreviews-clusterrolebinding",
		}, &rbacv1.ClusterRole{}); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating clusterrolebing", "clusterrolebing", msaName+"-subjectaccessreviews-clusterrolebinding")
			if err := syncer.client.Create(syncer.context, sarClusterRoleBindingObj); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}
