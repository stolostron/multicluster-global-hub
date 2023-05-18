// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
)

// AddClusterClaimController adds all the render controllers to the Manager.
func AddClusterClaimController(mgr ctrl.Manager) error {
	addControllerFunctions := []func(ctrl.Manager) error{
		StartClusterClaimController,
		StartHubClusterClaimController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}

func AddEventExporter(mgr ctrl.Manager, eventConfig, leafHubName string) error {
	return mgr.Add(&eventExporterController{
		kubeConfig:      mgr.GetConfig(),
		eventConfigFile: eventConfig,
	})
}
