package deployer

import (
	"context"
	"encoding/json"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployer is the interface for the kubernetes resource deployer
type Deployer interface {
	Deploy(unsObj *unstructured.Unstructured) error
}

type deployFunc func(*unstructured.Unstructured, *unstructured.Unstructured) error

// HoHDeployer is an implementation of Deployer interface
type HoHDeployer struct {
	client      client.Client
	deployFuncs map[string]deployFunc
}

// NewHoHDeployer creates a new HoHDeployer
func NewHoHDeployer(client client.Client) Deployer {
	deployer := &HoHDeployer{client: client}
	deployer.deployFuncs = map[string]deployFunc{
		"Deployment":         deployer.deployDeployment,
		"StatefulSet":        deployer.deployDeployment,
		"Service":            deployer.deployService,
		"ServiceAccount":     deployer.deployServiceAccount,
		"ConfigMap":          deployer.deployConfigMap,
		"Secret":             deployer.deploySecret,
		"Role":               deployer.deployRole,
		"RoleBinding":        deployer.deployRoleBinding,
		"ClusterRole":        deployer.deployClusterRole,
		"ClusterRoleBinding": deployer.deployClusterRoleBinding,
		"PodMonitor":         deployer.deployPodMonitor,
	}
	return deployer
}

func (d *HoHDeployer) Deploy(unsObj *unstructured.Unstructured) error {
	foundObj := &unstructured.Unstructured{}
	foundObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
	err := d.client.Get(
		context.TODO(),
		types.NamespacedName{Name: unsObj.GetName(), Namespace: unsObj.GetNamespace()},
		foundObj,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			return d.client.Create(context.TODO(), unsObj)
		}
		return err
	}

	// if resource has annotation skip-creation-if-exist: true, then it will not be updated
	metadata, ok := unsObj.Object["metadata"].(map[string]interface{})
	if ok {
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if ok && annotations != nil && annotations["skip-creation-if-exist"] != nil {
			if strings.ToLower(annotations["skip-creation-if-exist"].(string)) == "true" {
				return nil
			}
		}
	}

	deployFunction, ok := d.deployFuncs[foundObj.GetKind()]
	if ok {
		return deployFunction(unsObj, foundObj)
	} else {
		if !apiequality.Semantic.DeepDerivative(unsObj, foundObj) {
			// Retry logic to handle concurrent updates (e.g., KafkaUser) with exponential backoff
			return retry.RetryOnConflict(retry.DefaultRetry, func() error {
				// Get the latest version of the resource
				latestObj := &unstructured.Unstructured{}
				latestObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
				if err := d.client.Get(context.TODO(), types.NamespacedName{
					Name:      unsObj.GetName(),
					Namespace: unsObj.GetNamespace(),
				}, latestObj); err != nil {
					return err
				}

				unsObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
				unsObj.SetResourceVersion(latestObj.GetResourceVersion())
				return d.client.Update(context.TODO(), unsObj)
			})
		}
	}

	return nil
}

func (d *HoHDeployer) deployDeployment(desiredObj, existingObj *unstructured.Unstructured) error {
	// should not use DeepDerivative for typed object due to https://github.com/kubernetes/apimachinery/issues/110
	if !apiequality.Semantic.DeepDerivative(desiredObj.Object["spec"], existingObj.Object["spec"]) ||
		!apiequality.Semantic.DeepDerivative(desiredObj.GetLabels(), existingObj.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredObj.GetAnnotations(), existingObj.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredObj)
	}
	return nil
}

func (d *HoHDeployer) deployService(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingService := &corev1.Service{}
	err := json.Unmarshal(existingJSON, existingService)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredService := &corev1.Service{}
	err = json.Unmarshal(desiredJSON, desiredService)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredService.Spec, existingService.Spec) ||
		!apiequality.Semantic.DeepDerivative(desiredService.GetLabels(), existingService.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredService.GetAnnotations(), existingService.GetAnnotations()) {
		desiredService.ResourceVersion = existingService.ResourceVersion
		desiredService.Spec.ClusterIP = existingService.Spec.ClusterIP
		return d.client.Update(context.TODO(), desiredService)
	}

	return nil
}

func (d *HoHDeployer) deployServiceAccount(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingSA := &corev1.ServiceAccount{}
	err := json.Unmarshal(existingJSON, existingSA)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredSA := &corev1.ServiceAccount{}
	err = json.Unmarshal(desiredJSON, desiredSA)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredSA.Secrets, existingSA.Secrets) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.ImagePullSecrets, existingSA.ImagePullSecrets) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.GetLabels(), existingSA.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.GetAnnotations(), existingSA.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredSA)
	}

	return nil
}

func (d *HoHDeployer) deployPodMonitor(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingMonitor := &monitoringv1.PodMonitor{}
	err := json.Unmarshal(existingJSON, existingMonitor)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredMonitor := &monitoringv1.PodMonitor{}
	err = json.Unmarshal(desiredJSON, desiredMonitor)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredMonitor.Spec, existingMonitor.Spec) ||
		!apiequality.Semantic.DeepDerivative(desiredMonitor.GetLabels(), existingMonitor.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredMonitor.GetAnnotations(), existingMonitor.GetAnnotations()) {
		desiredMonitor.ResourceVersion = existingMonitor.ResourceVersion
		return d.client.Update(context.TODO(), desiredMonitor)
	}

	return nil
}

func (d *HoHDeployer) deployConfigMap(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingConfigMap := &corev1.ConfigMap{}
	err := json.Unmarshal(existingJSON, existingConfigMap)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredConfigMap := &corev1.ConfigMap{}
	err = json.Unmarshal(desiredJSON, desiredConfigMap)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredConfigMap.Data, existingConfigMap.Data) ||
		!apiequality.Semantic.DeepDerivative(desiredConfigMap.GetLabels(), existingConfigMap.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredConfigMap.GetAnnotations(), existingConfigMap.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredConfigMap)
	}

	return nil
}

func (d *HoHDeployer) deploySecret(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingSecret := &corev1.Secret{}
	err := json.Unmarshal(existingJSON, existingSecret)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredSecret := &corev1.Secret{}
	err = json.Unmarshal(desiredJSON, desiredSecret)
	if err != nil {
		return err
	}

	// handle secret stringData and data
	existingStrData := map[string]string{}
	for key, value := range existingSecret.Data {
		existingStrData[key] = string(value)
	}

	if !apiequality.Semantic.DeepDerivative(desiredSecret.StringData, existingStrData) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.Data, existingSecret.Data) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.GetLabels(), existingSecret.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.GetAnnotations(), existingSecret.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredSecret)
	}

	return nil
}

func (d *HoHDeployer) deployRole(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingRole := &rbacv1.Role{}
	err := json.Unmarshal(existingJSON, existingRole)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredRole := &rbacv1.Role{}
	err = json.Unmarshal(desiredJSON, desiredRole)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRole.Rules, existingRole.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredRole.GetLabels(), existingRole.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredRole.GetAnnotations(), existingRole.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredRole)
	}

	return nil
}

func (d *HoHDeployer) deployRoleBinding(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingRB := &rbacv1.RoleBinding{}
	err := json.Unmarshal(existingJSON, existingRB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredRB := &rbacv1.RoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredRB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRB.Subjects, existingRB.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.RoleRef, existingRB.RoleRef) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.GetLabels(), existingRB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.GetAnnotations(), existingRB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredRB)
	}

	return nil
}

func (d *HoHDeployer) deployClusterRole(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingCB := &rbacv1.ClusterRole{}
	err := json.Unmarshal(existingJSON, existingCB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCB := &rbacv1.ClusterRole{}
	err = json.Unmarshal(desiredJSON, desiredCB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredCB.Rules, existingCB.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.AggregationRule, existingCB.AggregationRule) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.GetLabels(), existingCB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.GetAnnotations(), existingCB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredCB)
	}

	return nil
}

func (d *HoHDeployer) deployClusterRoleBinding(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingCRB := &rbacv1.ClusterRoleBinding{}
	err := json.Unmarshal(existingJSON, existingCRB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCRB := &rbacv1.ClusterRoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredCRB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredCRB.Subjects, existingCRB.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.RoleRef, existingCRB.RoleRef) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.GetLabels(), existingCRB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.GetAnnotations(), existingCRB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredCRB)
	}

	return nil
}
