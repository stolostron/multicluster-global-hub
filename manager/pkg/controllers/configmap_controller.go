/*
Copyright 2024.

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

package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	RequeueDuration = 5 * time.Second
	LogLevelKey     = "logLevel"
)

type managerConfigMapController struct {
	client client.Client
}

func AddConfigMapController(mgr ctrl.Manager) error {
	configmapCtrl := &managerConfigMapController{
		client: mgr.GetClient(),
	}
	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == constants.GHConfigCMName
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(configMapPredicate).
		Complete(configmapCtrl)
}

func (c *managerConfigMapController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	log := logger.DefaultZapLogger()

	configMap := &corev1.ConfigMap{}
	if err := c.client.Get(ctx, request.NamespacedName, configMap); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: RequeueDuration}, fmt.Errorf("failed to get configmap: %w", err)
	}

	logLevel := configMap.Data[string(LogLevelKey)]
	if logLevel != "" {
		log.Infof("set the log level to: %s", logLevel)
		logger.SetLogLevel(logger.LogLevel(logLevel))
	}
	return ctrl.Result{}, nil
}
