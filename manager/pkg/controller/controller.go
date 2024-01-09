// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
)

func AddControllers(mgr ctrl.Manager) error {
	controllerFuncs := []func(ctrl.Manager) error{
		AddManagedHubClusterController,
	}
	for _, controllerFunc := range controllerFuncs {
		if err := controllerFunc(mgr); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
