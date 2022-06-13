package utils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	SERVICE_ACCOUNT_NAME              = "hoh-e2e-test-sa"
	SERVICE_ACCOUNT_ROLE_BINDING_NAME = "hoh-e2e-test-crb"
)

func CreateTestingRBAC(opt Options) error {
	// create new service account and new clusterrolebinding and bind the serviceaccount to cluster-admin clusterrole
	// then the bearer token can be retrieved from the secret of created serviceaccount
	testClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: SERVICE_ACCOUNT_ROLE_BINDING_NAME,
			Labels: map[string]string{
				"app": "hoh-e2e-test",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      SERVICE_ACCOUNT_NAME,
				Namespace: opt.HubCluster.Namespace,
			},
		},
	}
	if err := CreateClusterRoleBinding(opt, testClusterRoleBinding); err != nil {
		return fmt.Errorf("failed to create clusterrolebing for %s: %v", testClusterRoleBinding.GetName(), err)
	}

	testServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SERVICE_ACCOUNT_NAME,
			Namespace: opt.HubCluster.Namespace,
		},
	}
	if err := CreateServiceAccount(opt, testServiceAccount); err != nil {
		return fmt.Errorf("failed to create serviceaccount for %s: %v", testServiceAccount.GetName(), err)
	}
	return nil
}

func FetchBearerToken(opt Options) (string, error) {
	config, err := LoadConfig(
		opt.HubCluster.MasterURL,
		opt.HubCluster.KubeConfig,
		opt.HubCluster.KubeContext)
	if err != nil {
		return "", err
	}

	if config.BearerToken != "" {
		return config.BearerToken, nil
	}
	clients := NewTestClient(opt)
	kubeclient := clients.KubeClient()
	secretList, err := kubeclient.CoreV1().Secrets(opt.HubCluster.Namespace).List(context.TODO(), metav1.ListOptions{FieldSelector: "type=kubernetes.io/service-account-token"})
	if err != nil {
		return "", err
	}
	for _, secret := range secretList.Items {
		if len(secret.GetObjectMeta().GetAnnotations()) > 0 {
			annos := secret.GetObjectMeta().GetAnnotations()
			sa, saExists := annos["kubernetes.io/service-account.name"]
			_, createByExists := annos["kubernetes.io/created-by"]
			if saExists && !createByExists && sa == SERVICE_ACCOUNT_NAME {
				data := secret.Data
				if token, ok := data["token"]; ok {
					klog.V(5).Infof("token from secret: %s %s", secret.Namespace, &secret.Name)
					return string(token), nil
				}
			}
		}
	}
	return "", fmt.Errorf("failed to get bearer token")
}

func DeleteTestingRBAC(opt Options) error {
	clients := NewTestClient(opt)
	kubeclient := clients.KubeClient()
	if err := kubeclient.RbacV1().ClusterRoleBindings().Delete(context.TODO(), SERVICE_ACCOUNT_ROLE_BINDING_NAME, metav1.DeleteOptions{}); err != nil {
		return err
	}
	if err := kubeclient.CoreV1().ServiceAccounts(opt.HubCluster.Namespace).Delete(context.TODO(), SERVICE_ACCOUNT_NAME, metav1.DeleteOptions{}); err != nil {
		return err
	}
	return nil
}

func CreateClusterRoleBinding(opt Options, crb *rbacv1.ClusterRoleBinding) error {
	clients := NewTestClient(opt)
	_, err := clients.KubeClient().RbacV1().ClusterRoleBindings().Create(context.TODO(), crb, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(6).Infof("clusterrolebinding %s already exists, updating...", crb.GetName())
			_, err := clients.KubeClient().RbacV1().ClusterRoleBindings().Update(context.TODO(), crb, metav1.UpdateOptions{})
			return err
		}
		klog.Errorf("Failed to create cluster rolebinding %s due to %v", crb.GetName(), err)
		return err
	}
	return nil
}

func CreateServiceAccount(opt Options, sa *v1.ServiceAccount) error {
	clients := NewTestClient(opt)
	kubeclient := clients.KubeClient()
	_, err := kubeclient.CoreV1().ServiceAccounts(opt.HubCluster.Namespace).Create(context.TODO(), sa, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(6).Infof("serviceaccount %s already exists, skip", sa.GetName())
			_, err := kubeclient.CoreV1().ServiceAccounts(opt.HubCluster.Namespace).Update(context.TODO(), sa, metav1.UpdateOptions{})
			return err
		}
		klog.Errorf("Failed to create serviceaccount %s due to %v", sa.GetName(), err)
		return err
	}
	return nil
}
