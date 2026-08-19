// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transporter

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var hubHAACLLog = logger.DefaultZapLogger()

// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkausers,verbs=get;update;list;watch

type HubHAACLReconciler struct {
	mgr ctrl.Manager
	client.Client
}

func (r *HubHAACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if config.IsBYOKafka() {
		return ctrl.Result{}, nil
	}

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get MulticlusterGlobalHub for Hub HA ACL reconciler: %w", err)
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}

	cluster := &clusterv1.ManagedCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.syncHubHASpecWriteACL(ctx, mgh, req.Name, false)
		}
		return ctrl.Result{}, fmt.Errorf("get ManagedCluster %q for Hub HA ACL reconciler: %w", req.Name, err)
	}

	grant := cluster.DeletionTimestamp == nil &&
		cluster.Labels != nil &&
		cluster.Labels[constants.GHHubRoleLabelKey] == constants.GHHubRoleActive

	if err := r.syncHubHASpecWriteACL(ctx, mgh, req.Name, grant); err != nil {
		return ctrl.Result{}, err
	}
	if grant {
		hubHAACLLog.Infow("synced Hub HA spec write ACL", "activeHub", req.Name)
	}
	return ctrl.Result{}, nil
}

func (r *HubHAACLReconciler) syncHubHASpecWriteACL(
	ctx context.Context,
	mgh *operatorv1alpha4.MulticlusterGlobalHub,
	activeHub string,
	grant bool,
) error {
	if activeHub == "" {
		return nil
	}
	if err := protocol.SyncHubHASpecWriteACL(r.mgr, mgh, activeHub, grant, protocol.WithContext(ctx)); err != nil {
		return fmt.Errorf("failed to sync Hub HA spec write ACL for hub %q: %w", activeHub, err)
	}
	return nil
}

func (r *HubHAACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("hubha-transport-acl").
		For(&clusterv1.ManagedCluster{}).
		Complete(r)
}

func setupHubHAACLReconciler(mgr ctrl.Manager) error {
	if hubHAACLControllerStarted {
		return nil
	}
	reconciler := &HubHAACLReconciler{
		mgr:    mgr,
		Client: mgr.GetClient(),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup Hub HA ACL reconciler: %w", err)
	}
	hubHAACLControllerStarted = true
	return nil
}

var hubHAACLControllerStarted bool
