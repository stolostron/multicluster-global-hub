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
	"strings"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// +kubebuilder:rbac:groups=global-hub.open-cluster-management.io,resources=managedclustermigrations,verbs=get;list
// +kubebuilder:rbac:groups=global-hub.open-cluster-management.io,resources=managedclustermigrations,verbs=watch
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkausers,verbs=get;update;list;watch

type MigrationACLReconciler struct {
	mgr ctrl.Manager
	client.Client
}

func (r *MigrationACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if config.IsBYOKafka() {
		return ctrl.Result{}, nil
	}

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.Client)
	if err != nil || mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, err
	}

	migration := &migrationv1alpha1.ManagedClusterMigration{}
	if err := r.Get(ctx, req.NamespacedName, migration); err != nil {
		if apierrors.IsNotFound(err) {
			// Migration was deleted; re-sync all managed hub KafkaUsers so revoked ACLs
			// are cleaned up even though the trigger object's Spec.From is gone.
			return ctrl.Result{}, r.syncAllMigrationWriteACLs(ctx, mgh, req.Namespace)
		}
		return ctrl.Result{}, err
	}

	if err := r.syncMigrationWriteACLs(ctx, mgh, req.Namespace, migration.Spec.From); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MigrationACLReconciler) syncMigrationWriteACLs(
	ctx context.Context,
	mgh *operatorv1alpha4.MulticlusterGlobalHub,
	namespace string,
	triggerFromHub string,
) error {
	list := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := r.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list ManagedClusterMigration resources: %w", err)
	}

	needed := deployingSourceHubs(list)
	hubsToSync := hubsToSyncForMigration(triggerFromHub, needed)
	for _, fromHub := range hubsToSync {
		_, grant := needed[fromHub]
		if err := protocol.SyncMigrationWriteACL(r.mgr, mgh, fromHub, grant, protocol.WithContext(ctx)); err != nil {
			return fmt.Errorf("failed to sync migration write ACL for hub %q: %w", fromHub, err)
		}
	}
	return nil
}

func (r *MigrationACLReconciler) syncAllMigrationWriteACLs(
	ctx context.Context,
	mgh *operatorv1alpha4.MulticlusterGlobalHub,
	namespace string,
) error {
	list := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := r.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list ManagedClusterMigration resources: %w", err)
	}
	needed := deployingSourceHubs(list)

	kafkaUsers := &kafkav1beta2.KafkaUserList{}
	if err := r.List(ctx, kafkaUsers, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list KafkaUser resources: %w", err)
	}

	for i := range kafkaUsers.Items {
		fromHub, ok := managedHubFromKafkaUser(&kafkaUsers.Items[i])
		if !ok {
			continue
		}
		_, grant := needed[fromHub]
		if err := protocol.SyncMigrationWriteACL(r.mgr, mgh, fromHub, grant, protocol.WithContext(ctx)); err != nil {
			return fmt.Errorf("failed to sync migration write ACL for hub %q: %w", fromHub, err)
		}
	}
	return nil
}

func deployingSourceHubs(list *migrationv1alpha1.ManagedClusterMigrationList) map[string]struct{} {
	needed := make(map[string]struct{})
	for i := range list.Items {
		mcm := &list.Items[i]
		if mcm.DeletionTimestamp != nil {
			continue
		}
		if mcm.Status.Phase == migrationv1alpha1.PhaseDeploying {
			needed[mcm.Spec.From] = struct{}{}
		}
	}
	return needed
}

func hubsToSyncForMigration(fromHub string, needed map[string]struct{}) []string {
	hubs := sets.NewString()
	if fromHub != "" {
		hubs.Insert(fromHub)
	}
	for hub := range needed {
		hubs.Insert(hub)
	}
	return hubs.UnsortedList()
}

func managedHubFromKafkaUser(user *kafkav1beta2.KafkaUser) (string, bool) {
	if user.Name == protocol.DefaultGlobalHubKafkaUserName {
		return "", false
	}
	if !strings.HasSuffix(user.Name, "-kafka-user") {
		return "", false
	}
	fromHub := strings.TrimSuffix(user.Name, "-kafka-user")
	if fromHub == constants.LocalClusterName || fromHub == config.GetLocalClusterName() {
		return "", false
	}
	return fromHub, true
}

func (r *MigrationACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("migration-transport-acl").
		For(&migrationv1alpha1.ManagedClusterMigration{}).
		Complete(r)
}

func setupMigrationACLReconciler(mgr ctrl.Manager) error {
	if migrationACLControllerStarted {
		return nil
	}
	reconciler := &MigrationACLReconciler{
		mgr:    mgr,
		Client: mgr.GetClient(),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return err
	}
	migrationACLControllerStarted = true
	return nil
}

var migrationACLControllerStarted bool
