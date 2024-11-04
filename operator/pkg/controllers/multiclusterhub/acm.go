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

package multiclusterhub

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const MULTICLUSTER_HUB_CRD_NAME = "multiclusterhubs.operator.open-cluster-management.io"

var (
	log                   = logger.DefaultZapLogger()
	acmConstrollerStarted = false
)

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update

type MulticlusterhubController struct {
	manager.Manager
}

func (r *MulticlusterhubController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}
	err := r.GetClient().Get(ctx, client.ObjectKeyFromObject(mch), mch)
	if err != nil && errors.IsNotFound(err) {
		config.SetACMResourceReady(false)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		log.Errorf("failed to get the multiclusterhub: %v", err)
		return ctrl.Result{}, err
	}

	if mch.Status.Phase != mchv1.HubRunning {
		config.SetACMResourceReady(false)
		err = config.UpdateCondition(ctx, r.GetClient(), config.GetMGHNamespacedName(),
			metav1.Condition{
				Type:    config.CONDITION_TYPE_ACM_READY,
				Status:  config.CONDITION_STATUS_FALSE,
				Reason:  config.CONDITION_REASON_ACM_NOT_READY,
				Message: config.CONDITION_MESSAGE_ACM_NOT_READY,
			}, v1alpha4.GlobalHubError)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// MCH is running, update the mgh condition to trigger the main(meta) loop
	config.SetACMResourceReady(true)
	err = config.UpdateCondition(ctx, r.GetClient(), config.GetMGHNamespacedName(),
		metav1.Condition{
			Type:    config.CONDITION_TYPE_ACM_READY,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  config.CONDITION_REASON_ACM_READY,
			Message: config.CONDITION_MESSAGE_ACM_READY,
		}, v1alpha4.GlobalHubError)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func AddMulticlusterHubController(opts config.ControllerOption) error {
	if acmConstrollerStarted {
		return nil
	}
	mch := &MulticlusterhubController{Manager: opts.Manager}
	err := ctrl.NewControllerManagedBy(opts.Manager).
		Named("acm-controller").
		WatchesMetadata(
			&apiextensionsv1.CustomResourceDefinition{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetName() == MULTICLUSTER_HUB_CRD_NAME
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetName() == MULTICLUSTER_HUB_CRD_NAME
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			}),
		).
		Complete(mch)
	if err != nil {
		return err
	}
	acmConstrollerStarted = true
	return nil
}
