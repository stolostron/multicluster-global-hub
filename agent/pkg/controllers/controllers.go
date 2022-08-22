// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"fmt"

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

func InitResources(mgr ctrl.Manager) error {
	initControllerFunctions := []func(ctrl.Manager) error{
		InitClusterRole,
		InitClusterRoleBinding,
	}

	for _, initControllerFunction := range initControllerFunctions {
		if err := initControllerFunction(mgr); err != nil {
			return fmt.Errorf("failed to init controller: %w", err)
		}
	}

	return nil
}
