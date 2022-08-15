// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package status

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/status/controller"
)

// AddStatusControllers adds all the status controllers to the Manager.
func AddStatusControllers(mgr ctrl.Manager) error {
	addControllerFunctions := []func(ctrl.Manager) error{
		controller.AddPlacementBindingController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
