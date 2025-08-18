/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

const (
	CONDITION_STATUS_TRUE    = "True"
	CONDITION_STATUS_FALSE   = "False"
	CONDITION_STATUS_UNKNOWN = "Unknown"
)

const (
	COMPONENTS_AVAILABLE = "Available"
	COMPONENTS_CREATING  = "ComponentsCreating"
	COMPONENTS_NOT_READY = "ComponentsNotReady"
	RECONCILE_ERROR      = "ReconcileError"
	MESSAGE_WAIT_CREATED = "Waiting for the resource to be created"
	MESSAGE_WAIT_READY   = "Waiting for the component to be ready"
	COMPONENTS_DEPLOYED  = "Component %s has been deployed"
)

const (
	COMPONENTS_KAFKA_NAME            = "kafka"
	COMPONENTS_POSTGRES_NAME         = "multicluster-global-hub-postgresql"
	COMPONENTS_MANAGER_NAME          = "multicluster-global-hub-manager"
	COMPONENTS_AGENT_NAME            = "multicluster-global-hub-agent"
	COMPONENTS_GRAFANA_NAME          = "multicluster-global-hub-grafana"
	COMPONENTS_INVENTORY_API_NAME    = "inventory-api"
	COMPONENTS_SPICEDB_OPERATOR_NAME = "spicedb-operator"
)

// NOTE: the status of Inventory deploy
const (
	CONDITION_TYPE_INVENTORY_AVAILABLE = "InventoryAvailable"
)

// NOTE: Kafka related condition
const (
	CONDITION_TYPE_KAFKA          = "Kafka"
	CONDITION_REASON_KAFKA_READY  = "KafkaClusterReady"
	CONDITION_MESSAGE_KAFKA_READY = "Kafka cluster is ready"
)

// NOTE: the status of Data Retention can be True or False
const (
	CONDITION_TYPE_DATABASE                  = "Database"
	CONDITION_TYPE_RETENTION_PARSED          = "DataRetentionParsed"
	CONDITION_REASON_RETENTION_PARSED        = "DataRetentionParsed"
	CONDITION_REASON_RETENTION_PARSED_FAILED = "DataRetentionParsedFailed"
)

// NOTE: the status of ManagerDeployed can only be True; otherwise there is no condition
const (
	MINIMUM_REPLICAS_AVAILABLE          = "MinimumReplicasAvailable"
	MINIMUM_REPLICAS_UNAVAILABLE        = "MinimumReplicasUnavailable"
	CONDITION_REASON_MANAGER_AVAILABLE  = "DeployedButNotReady"
	CONDITION_MESSAGE_MANAGER_AVAILABLE = "The multicluster global hub manager has been deployed"
)

const (
	CONDITION_TYPE_GLOBALHUB_READY        = "Ready"
	CONDITION_REASON_GLOBALHUB_READY      = "MulticlusterGlobalHubReady"
	CONDITION_REASON_GLOBALHUB_NOT_READY  = "MulticlusterGlobalHubNotReady"
	CONDITION_MESSAGE_GLOBALHUB_READY     = "The multicluster global hub is ready, and all the components are available"
	CONDITION_MESSAGE_GLOBALHUB_NOT_READY = "The multicluster global hub is not ready, waiting for all the components to be available"

	CONDITION_REASON_GLOBALHUB_FAILED     = "MulticlusterGlobalHubFailed"
	CONDITION_REASON_GLOBALHUB_UNINSTALL  = "MulticlusterGlobalUninstalling"
	CONDITION_MESSAGE_GLOBALHUB_UNINSTALL = "The multicluster global hub is uninstalling"
)

const (
	CONDITION_TYPE_ACM_RESOURCE_READY        = "ACMResourceReady"
	CONDITION_REASON_ACM_RESOURCE_READY      = "ACMResourceReady"
	CONDITION_REASON_ACM_RESOURCE_NOT_READY  = "ACMResourceNotReady"
	CONDITION_MESSAGE_ACM_RESOURCE_READY     = "The ACM Resources are ready"
	CONDITION_MESSAGE_ACM_RESOURCE_NOT_READY = "The ACM Resources are not ready, waiting for installing"
)

const (
	CONDITION_TYPE_BACKUP             = "BackupLabelAdded"
	CONDITION_REASON_BACKUP           = "BackupLabelAdded"
	CONDITION_MESSAGE_BACKUP          = "Added backup label to the global hub resources"
	CONDITION_REASON_BACKUP_DISABLED  = "BackupDisabled"
	CONDITION_MESSAGE_BACKUP_DISABLED = "Backup is disabled in RHACM"
)

type GetComponentStatus func(ctx context.Context,
	c client.Client,
	namespace string,
	componentName string) (ComponentStatus, error)

// SetConditionFunc is function type that receives the concrete condition method
type SetConditionFunc func(ctx context.Context, c client.Client,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	status metav1.ConditionStatus) error

func ContainsCondition(mgh *globalhubv1alpha4.MulticlusterGlobalHub, typeName string) bool {
	output := false
	for _, condition := range mgh.Status.Conditions {
		if condition.Type == typeName {
			output = true
		}
	}
	return output
}

func ContainConditionStatus(mgh *globalhubv1alpha4.MulticlusterGlobalHub, typeName string,
	status metav1.ConditionStatus,
) bool {
	output := false
	for _, condition := range mgh.Status.Conditions {
		if condition.Type == typeName && condition.Status == status {
			output = true
		}
	}
	return output
}

func UpdateCondition(ctx context.Context, c client.Client, mghNamespacedName types.NamespacedName,
	cond metav1.Condition, phase globalhubv1alpha4.GlobalHubPhaseType,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curmgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
		err := c.Get(ctx, mghNamespacedName, curmgh)
		if err != nil {
			return err
		}
		conditionUpdated, desiredConditions := NeedUpdateConditions(curmgh.Status.Conditions, cond)
		if conditionUpdated {
			curmgh.Status.Conditions = desiredConditions
		}
		var phaseUpdated bool
		if phase != "" && phase != curmgh.Status.Phase {
			phaseUpdated = true
			curmgh.Status.Phase = phase
		}

		if !phaseUpdated && !conditionUpdated {
			return nil
		}
		err = c.Status().Update(context.TODO(), curmgh)
		return err
	})
}

func UpdateConditionWithErr(ctx context.Context, c client.Client, err error,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	conditionType string, conditionReason string, conditionMessage string,
) {
	if err != nil {
		err = UpdateCondition(ctx, c, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      mgh.Name,
		}, metav1.Condition{
			Type:    conditionType,
			Status:  CONDITION_STATUS_FALSE,
			Reason:  conditionReason,
			Message: err.Error(),
		}, globalhubv1alpha4.GlobalHubError)
		if err != nil {
			log.Errorf("failed to update mgh condition:%v", err)
		}
		return
	}
	err = UpdateCondition(ctx, c, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      mgh.Name,
	}, metav1.Condition{
		Type:    conditionType,
		Reason:  conditionReason,
		Status:  CONDITION_STATUS_TRUE,
		Message: conditionMessage,
	}, "")
	if err != nil {
		log.Errorf("failed to update mgh condition:%v", err)
	}
}

// NeedUpdateConditions check if the condition need update and return the desired conditions
func NeedUpdateConditions(conditions []metav1.Condition,
	cond metav1.Condition,
) (bool, []metav1.Condition) {
	isExist := false
	cond.LastTransitionTime = metav1.Time{Time: time.Now()}
	for i, curCon := range conditions {
		if curCon.Type == cond.Type {
			isExist = true
			if curCon.Status == cond.Status &&
				curCon.Reason == cond.Reason &&
				curCon.Message == cond.Message {
				return false, conditions
			}
			conditions[i] = cond
			break
		}
	}
	if !isExist {
		conditions = append(conditions, cond)
	}
	return true, conditions
}

func UpdateMGHComponent(ctx context.Context,
	c client.Client,
	desiredComponent globalhubv1alpha4.StatusCondition,
	forceUpdate bool,
) error {
	now := metav1.Time{Time: time.Now()}
	desiredComponent.LastTransitionTime = now
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curmgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
		err := c.Get(ctx, GetMGHNamespacedName(), curmgh)
		if err != nil {
			return err
		}
		if curmgh.Status.Components == nil {
			curmgh.Status.Components = map[string]globalhubv1alpha4.StatusCondition{
				desiredComponent.Name: desiredComponent,
			}
		} else {
			originComponent := curmgh.Status.Components[desiredComponent.Name]
			originComponent.LastTransitionTime = now
			if !forceUpdate && reflect.DeepEqual(desiredComponent, originComponent) {
				return nil
			}
		}

		curmgh.Status.Components[desiredComponent.Name] = desiredComponent
		return c.Status().Update(ctx, curmgh)
	})
}

func GetStatefulSetComponentStatus(ctx context.Context, c client.Client,
	namespace, name string,
) globalhubv1alpha4.StatusCondition {
	statefulset := &appsv1.StatefulSet{}

	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, statefulset)
	if err != nil {
		if errors.IsNotFound(err) {
			return globalhubv1alpha4.StatusCondition{
				Kind:    "StatefulSet",
				Name:    name,
				Type:    COMPONENTS_AVAILABLE,
				Reason:  COMPONENTS_NOT_READY,
				Status:  CONDITION_STATUS_FALSE,
				Message: fmt.Sprintf("waiting statefulset %s created", name),
			}
		}
		log.Errorf("failed to get statefulset, err: %v", err)
		return globalhubv1alpha4.StatusCondition{
			Kind:    "StatefulSet",
			Name:    name,
			Type:    COMPONENTS_AVAILABLE,
			Reason:  COMPONENTS_NOT_READY,
			Status:  CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("failed to get statefulset, err: %v", err),
		}
	}
	if statefulset.Status.AvailableReplicas == *statefulset.Spec.Replicas {
		return globalhubv1alpha4.StatusCondition{
			Kind:    "StatefulSet",
			Name:    name,
			Type:    COMPONENTS_AVAILABLE,
			Reason:  MINIMUM_REPLICAS_AVAILABLE,
			Status:  CONDITION_STATUS_TRUE,
			Message: fmt.Sprintf(COMPONENTS_DEPLOYED, name),
		}
	}
	return globalhubv1alpha4.StatusCondition{
		Kind:    "StatefulSet",
		Name:    name,
		Type:    COMPONENTS_AVAILABLE,
		Reason:  MINIMUM_REPLICAS_UNAVAILABLE,
		Status:  CONDITION_STATUS_FALSE,
		Message: fmt.Sprintf("Component %s has been deployed but is not ready", name),
	}
}

func GetDeploymentComponentStatus(ctx context.Context, c client.Client,
	namespace, name string,
) globalhubv1alpha4.StatusCondition {
	deployment := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return globalhubv1alpha4.StatusCondition{
				Kind:    "Deployment",
				Name:    name,
				Type:    COMPONENTS_AVAILABLE,
				Reason:  COMPONENTS_NOT_READY,
				Status:  CONDITION_STATUS_FALSE,
				Message: fmt.Sprintf("waiting deployment %s created", name),
			}
		}
		log.Errorf("failed to get deployment, err: %v", err)
		return globalhubv1alpha4.StatusCondition{
			Kind:    "Deployment",
			Name:    name,
			Type:    COMPONENTS_AVAILABLE,
			Reason:  COMPONENTS_NOT_READY,
			Status:  CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("failed to get deployment, err: %v", err),
		}
	}
	if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
		return globalhubv1alpha4.StatusCondition{
			Kind:    "Deployment",
			Name:    name,
			Type:    COMPONENTS_AVAILABLE,
			Reason:  MINIMUM_REPLICAS_AVAILABLE,
			Status:  CONDITION_STATUS_TRUE,
			Message: fmt.Sprintf(COMPONENTS_DEPLOYED, name),
		}
	}
	return globalhubv1alpha4.StatusCondition{
		Kind:    "Deployment",
		Name:    name,
		Type:    COMPONENTS_AVAILABLE,
		Reason:  MINIMUM_REPLICAS_UNAVAILABLE,
		Status:  CONDITION_STATUS_FALSE,
		Message: fmt.Sprintf("Component %s has been deployed but is not ready", name),
	}
}

func GetComponentStatusWithReconcileError(ctx context.Context, c client.Client,
	namespace, name string, reconcileErr error,
) globalhubv1alpha4.StatusCondition {
	availableType := COMPONENTS_AVAILABLE
	if reconcileErr != nil {
		return globalhubv1alpha4.StatusCondition{
			Kind:    "Deployment",
			Name:    name,
			Type:    availableType,
			Status:  CONDITION_STATUS_FALSE,
			Reason:  RECONCILE_ERROR,
			Message: reconcileErr.Error(),
		}
	}
	return GetDeploymentComponentStatus(ctx, c, namespace, name)
}
