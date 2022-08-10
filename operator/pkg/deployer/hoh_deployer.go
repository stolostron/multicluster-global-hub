package deployer

import (
	"context"
	"encoding/json"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
		"Deployment":               deployer.deployDeployment,
		"Service":                  deployer.deployService,
		"ConfigMap":                deployer.deployConfigMap,
		"Secret":                   deployer.deploySecret,
		"ClusterRole":              deployer.deployClusterRole,
		"ClusterRoleBinding":       deployer.deployClusterRoleBinding,
		"CustomResourceDefinition": deployer.deployCRD,
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
	}
	return nil
}

func (d *HoHDeployer) deployDeployment(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingDepoly := &appsv1.Deployment{}
	err := json.Unmarshal(existingJSON, existingDepoly)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredDepoly := &appsv1.Deployment{}
	err = json.Unmarshal(desiredJSON, desiredDepoly)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredDepoly, existingDepoly) {
		return d.client.Update(context.TODO(), desiredDepoly)
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

	if !apiequality.Semantic.DeepDerivative(desiredService, existingService) {
		desiredService.ObjectMeta.ResourceVersion =
			existingService.ObjectMeta.ResourceVersion
		desiredService.Spec.ClusterIP = existingService.Spec.ClusterIP
		return d.client.Update(context.TODO(), desiredService)
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

	if !apiequality.Semantic.DeepDerivative(desiredConfigMap, existingConfigMap) {
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

	if !apiequality.Semantic.DeepDerivative(desiredSecret, existingSecret) {
		return d.client.Update(context.TODO(), desiredSecret)
	}

	return nil
}

func (d *HoHDeployer) deployClusterRole(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingClusterRole := &rbacv1.ClusterRole{}
	err := json.Unmarshal(existingJSON, existingClusterRole)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredClusterRole := &rbacv1.ClusterRole{}
	err = json.Unmarshal(desiredJSON, desiredClusterRole)
	if err != nil {
		return err
	}

	// if !apiequality.Semantic.DeepDerivative(desiredClusterRole.Rules, existingClusterRole.Rules) ||
	// 	!apiequality.Semantic.DeepDerivative(desiredClusterRole.AggregationRule, existingClusterRole.AggregationRule) {
	if !apiequality.Semantic.DeepDerivative(desiredClusterRole, existingClusterRole) {
		return d.client.Update(context.TODO(), desiredClusterRole)
	}

	return nil
}

func (d *HoHDeployer) deployClusterRoleBinding(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := json.Unmarshal(existingJSON, existingClusterRoleBinding)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredClusterRoleBinding)
	if err != nil {
		return err
	}

	// if !apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.Subjects, existingClusterRoleBinding.Subjects) ||
	// 	!apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.RoleRef, existingClusterRoleBinding.RoleRef) {
	if !apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding, existingClusterRoleBinding) {
		return d.client.Update(context.TODO(), desiredClusterRoleBinding)
	}

	return nil
}

func (d *HoHDeployer) deployCRD(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingCRD := &apiextensionsv1.CustomResourceDefinition{}
	err := json.Unmarshal(existingJSON, existingCRD)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCRD := &apiextensionsv1.CustomResourceDefinition{}
	err = json.Unmarshal(desiredJSON, desiredCRD)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredCRD, existingCRD) {
		return d.client.Update(context.TODO(), desiredCRD)
	}

	return nil
}
