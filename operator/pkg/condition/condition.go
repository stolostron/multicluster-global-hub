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

package condition

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
)

type ICondition interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

const (
	CONDITION_STATUS_TRUE    = "True"
	CONDITION_STATUS_FALSE   = "False"
	CONDITION_STATUS_UNKNOWN = "Unknown"
)

// NOTE: the status of GrafanaInitialized can be True or False
const (
	CONDITION_TYPE_GRAFANA_DEPLOY    = "GrafanaDeployed"
	CONDITION_REASON_GRAFANA_DEPLOY  = "GrafanaDeployed"
	CONDITION_MESSAGE_GRAFANA_DEPLOY = "Multicluster Global Hub Grafana has been deployed"
)

// NOTE: the status of DatabaseInitialized can be True or False
const (
	CONDITION_TYPE_DATABASE_INIT    = "DatabaseInitialized"
	CONDITION_REASON_DATABASE_INIT  = "DatabaseInitialized"
	CONDITION_MESSAGE_DATABASE_INIT = "Database has been initialized"
)

// NOTE: the status of TransportInitialized can be True or False
const (
	CONDITION_TYPE_TRANSPORT_INIT    = "TransportInitialized"
	CONDITION_REASON_TRANSPORT_INIT  = "TransportInitialized"
	CONDITION_MESSAGE_TRANSPORT_INIT = "Transport has been initialized"
)

// NOTE: the status of ManagerDeployed can only be True; otherwise there is no condition
const (
	CONDITION_TYPE_MANAGER_DEPLOY    = "ManagerDeployed"
	CONDITION_REASON_MANAGER_DEPLOY  = "ManagerDeployed"
	CONDITION_MESSAGE_MANAGER_DEPLOY = "Multicluster Global Hub Manager has been deployed"
)

// NOTE: the status of LeafHubDeployed can only be True; otherwise there is no condition
const (
	CONDITION_TYPE_LEAFHUB_DEPLOY           = "LeafHubDeployed"
	CONDITION_REASON_LEAFHUB_DEPLOY         = "LeafHubDeployed"
	CONDITION_MESSAGE_LEAFHUB_DEPLOY        = "Leaf Hub Deployed"
	CONDITION_REASON_LEAFHUB_DEPLOY_FAILED  = "LeafHubDeployFailed"
	CONDITION_MESSAGE_LEAFHUB_DEPLOY_FAILED = "Leaf Hub Deployed FAILED"
)

// SetConditionFunc is function type that receives the concrete condition method
type SetConditionFunc func(ctx context.Context, c client.Client,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
	status metav1.ConditionStatus) error

func FailToSetConditionError(condition string, err error) error {
	return fmt.Errorf("failed to set condition(%s): %w", condition, err)
}

func SetConditionGrafanaDeployed(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	status metav1.ConditionStatus,
) error {
	return SetCondition(ctx, c, mgh, CONDITION_TYPE_GRAFANA_DEPLOY, status, CONDITION_REASON_GRAFANA_DEPLOY,
		CONDITION_MESSAGE_GRAFANA_DEPLOY)
}

func SetConditionDatabaseInit(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	status metav1.ConditionStatus,
) error {
	return SetCondition(ctx, c, mgh, CONDITION_TYPE_DATABASE_INIT, status,
		CONDITION_REASON_DATABASE_INIT, CONDITION_MESSAGE_DATABASE_INIT)
}

func SetConditionTransportInit(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	status metav1.ConditionStatus,
) error {
	return SetCondition(ctx, c, mgh, CONDITION_TYPE_TRANSPORT_INIT, status,
		CONDITION_REASON_TRANSPORT_INIT, CONDITION_MESSAGE_TRANSPORT_INIT)
}

func SetConditionManagerDeployed(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	status metav1.ConditionStatus,
) error {
	return SetCondition(ctx, c, mgh, CONDITION_TYPE_MANAGER_DEPLOY, status,
		CONDITION_REASON_MANAGER_DEPLOY, CONDITION_MESSAGE_MANAGER_DEPLOY)
}

func SetConditionLeafHubDeployed(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	clusterName string, status metav1.ConditionStatus,
) error {
	reason := CONDITION_REASON_LEAFHUB_DEPLOY
	message := CONDITION_MESSAGE_LEAFHUB_DEPLOY
	if status == CONDITION_STATUS_FALSE {
		reason = CONDITION_REASON_LEAFHUB_DEPLOY_FAILED
		message = fmt.Sprintf("%s-%s", CONDITION_MESSAGE_LEAFHUB_DEPLOY_FAILED, clusterName)
	}

	return SetCondition(ctx, c, mgh, CONDITION_TYPE_LEAFHUB_DEPLOY, status, reason, message)
}

func SetCondition(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub, typeName string,
	status metav1.ConditionStatus, reason string, message string,
) error {
	if !ContainsCondition(mgh, typeName) {
		return AppendCondition(ctx, c, mgh, typeName,
			status, reason, message)
	} else {
		currentStatus := GetConditionStatus(mgh, typeName)
		if currentStatus != status {
			err := DeleteCondition(ctx, c, mgh, typeName, reason)
			if err != nil {
				return err
			}
			return AppendCondition(ctx, c, mgh, typeName, status,
				reason, message)
		}
	}
	return nil
}

func ContainsCondition(mgh *operatorv1alpha2.MulticlusterGlobalHub, typeName string) bool {
	output := false
	for _, condition := range mgh.Status.Conditions {
		if condition.Type == typeName {
			output = true
		}
	}
	return output
}

func ContainConditionStatus(mgh *operatorv1alpha2.MulticlusterGlobalHub, typeName string,
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

func ContainConditionStatusReason(mgh *operatorv1alpha2.MulticlusterGlobalHub,
	typeName, reason string, status metav1.ConditionStatus,
) bool {
	output := false
	for _, condition := range mgh.Status.Conditions {
		if condition.Type == typeName && condition.Reason == reason && condition.Status == status {
			output = true
		}
	}
	return output
}

func GetConditionStatus(mgh *operatorv1alpha2.MulticlusterGlobalHub, typeName string) metav1.ConditionStatus {
	var output metav1.ConditionStatus = CONDITION_STATUS_UNKNOWN
	for _, condition := range mgh.Status.Conditions {
		if condition.Type == typeName {
			output = condition.Status
		}
	}
	return output
}

func DeleteCondition(ctx context.Context, c client.Client, mgh *operatorv1alpha2.MulticlusterGlobalHub,
	typeName string, reason string,
) error {
	newConditions := make([]metav1.Condition, 0)
	for _, condition := range mgh.Status.Conditions {
		if condition.Type != typeName && condition.Reason != reason {
			newConditions = append(newConditions, condition)
		}
	}
	mgh.Status.Conditions = newConditions
	err := c.Status().Update(ctx, mgh)
	if err != nil {
		return fmt.Errorf("failed to update hoh mgh status condition: %v", err)
	}
	return nil
}

func AppendCondition(ctx context.Context, c client.Client, object client.Object, typeName string,
	status metav1.ConditionStatus, reason string, message string,
) error {
	conditions, ok := (object).(ICondition)
	if ok {
		condition := metav1.Condition{
			Type: typeName, Status: status, Reason: reason,
			Message: message, LastTransitionTime: metav1.Time{Time: time.Now()},
		}
		conditions.SetConditions(append(conditions.GetConditions(), condition))
		err := c.Status().Update(ctx, object)
		if err != nil {
			return fmt.Errorf("failed to update status condition: %v", err)
		}
	} else {
		return fmt.Errorf("status condition cannot be set, resource does not support Conditions")
	}
	return nil
}
