// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AddControllers adds all the render controllers to the Manager.
func AddControllers(mgr ctrl.Manager) error {
	addControllerFunctions := []func(ctrl.Manager) error{
		AddClusterRoleController,
		AddClusterRoleBindingController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}

func InitResources(ctx context.Context, kubeClient *kubernetes.Clientset) error {
	initControllerFunctions := []func(context.Context, *kubernetes.Clientset) error{
		InitClusterRole,
		InitClusterRoleBinding,
	}

	for _, initControllerFunction := range initControllerFunctions {
		if err := initControllerFunction(ctx, kubeClient); err != nil {
			return fmt.Errorf("failed to execute init function: %w", err)
		}
	}

	return nil
}
