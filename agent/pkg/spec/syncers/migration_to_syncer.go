// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	KlusterletManifestWorkSuffix = "-klusterlet"
)

var (
	log                = logger.DefaultZapLogger()
	registeringTimeout = 10 * time.Minute // the registering stage timeout should less than migration timeout
)

type MigrationTargetSyncer struct {
	client             client.Client
	transportClient    transport.TransportClient
	transportConfig    *transport.TransportInternalConfig
	bundleVersion      *eventversion.Version
	currentMigrationId string
}

func NewMigrationTargetSyncer(client client.Client,
	transportClient transport.TransportClient, transportConfig *transport.TransportInternalConfig,
) *MigrationTargetSyncer {
	return &MigrationTargetSyncer{
		client:          client,
		transportClient: transportClient,
		transportConfig: transportConfig,
		bundleVersion:   eventversion.NewVersion(),
	}
}

func (s *MigrationTargetSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	log.Infof("received migration event from %s", evt.Source())

	var err error
	var stage string

	defer func() {
		if stage == "" || s.currentMigrationId == "" {
			log.Warnf("stage(%s) or migrationId(%s) is empty ", stage, s.currentMigrationId)
			return
		}
		errMessage := ""
		if err != nil {
			errMessage = err.Error()
		}
		err = ReportMigrationStatus(
			cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
			s.transportClient,
			&migration.ManagedClusterMigrationBundle{
				MigrationId: s.currentMigrationId,
				Stage:       stage,
				ErrMessage:  errMessage,
			}, s.bundleVersion)
		if err != nil {
			log.Errorf("failed to report migration status: %v", err)
		}
	}()

	// Handle direct deploying events from source hub (not from global hub)
	if evt.Source() != constants.CloudEventGlobalHubClusterName {
		stage = migrationv1alpha1.PhaseDeploying
		err = s.deploying(ctx, evt)
		if err != nil {
			return fmt.Errorf("failed to handle deploying event: %w", err)
		}
		return nil
	}

	// Parse migration event from global hub
	event := &migration.ManagedClusterMigrationToEvent{}
	if err := json.Unmarshal(evt.Data(), event); err != nil {
		return fmt.Errorf("failed to unmarshal migration event: %w", err)
	}

	log.Debugf("received migration event: migrationId=%s, stage=%s", event.MigrationId, event.Stage)
	if event.MigrationId == "" {
		return fmt.Errorf("migrationId is required but not provided in event")
	}

	stage = event.Stage

	// Set current migration ID and reset bundle version for initializing stage
	if event.Stage == migrationv1alpha1.PhaseInitializing {
		s.currentMigrationId = event.MigrationId
		s.bundleVersion.Reset()
	}

	// Check if migration ID matches for all other stages
	if s.currentMigrationId != event.MigrationId {
		log.Infof("ignoring migration event %s, current migrationId is %s", event.MigrationId, s.currentMigrationId)
		return nil
	}

	err = s.handleMigrationStage(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to handle migration stage: %w", err)
	}
	return nil
}

func (s *MigrationTargetSyncer) SetMigrationID(id string) {
	s.currentMigrationId = id
}

// handleMigrationStage processes different migration stages using switch statement
func (s *MigrationTargetSyncer) handleMigrationStage(ctx context.Context, event *migration.ManagedClusterMigrationToEvent) error {
	switch event.Stage {
	case migrationv1alpha1.PhaseInitializing:
		return s.executeStage(ctx, event, s.initializing)
	case migrationv1alpha1.PhaseRegistering:
		return s.executeStage(ctx, event, s.registering)
	case migrationv1alpha1.PhaseCleaning:
		return s.executeStage(ctx, event, s.cleaning)
	case migrationv1alpha1.PhaseRollbacking:
		return s.executeStage(ctx, event, s.rollbacking)
	default:
		log.Warnf("unknown migration stage: %s", event.Stage)
		return fmt.Errorf("unknown migration stage: %s", event.Stage)
	}
}

// executeStage executes a migration stage with consistent logging
func (s *MigrationTargetSyncer) executeStage(ctx context.Context, event *migration.ManagedClusterMigrationToEvent,
	stageFunc func(context.Context, *migration.ManagedClusterMigrationToEvent) error,
) error {
	log.Infof("migration %s started: migrationId=%s, clusters=%v", event.Stage, event.MigrationId, event.ManagedClusters)

	if err := stageFunc(ctx, event); err != nil {
		log.Errorf("migration %s failed: migrationId=%s, error=%v",
			event.Stage, event.MigrationId, err)
		return err
	}

	log.Infof("migration %s completed: migrationId=%s", event.Stage, event.MigrationId)
	return nil
}

func (s *MigrationTargetSyncer) cleaning(ctx context.Context,
	event *migration.ManagedClusterMigrationToEvent,
) error {
	msaName := event.ManagedServiceAccountName

	// Clean up RBAC resources created for migration
	if err := s.cleanupMigrationRBAC(ctx, msaName); err != nil {
		log.Errorf("failed to cleanup migration RBAC resources: %v", err)
		return err
	}
	return nil
}

// registering watches the migrated managed clusters
func (s *MigrationTargetSyncer) registering(ctx context.Context,
	event *migration.ManagedClusterMigrationToEvent,
) error {
	if len(event.ManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters found in migration event: %s", event.MigrationId)
	}

	errMessage := ""
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, registeringTimeout, true,
		func(context.Context) (done bool, err error) {
			notReadyClusters := []string{}
			for _, clusterName := range event.ManagedClusters {
				if err := s.checkClusterManifestWork(ctx, clusterName); err != nil {
					log.Debugf("cluster %s not ready: %v", clusterName, err)
					notReadyClusters = append(notReadyClusters, fmt.Sprintf("cluster(%s): %s", clusterName, err.Error()))
				}
			}

			if len(notReadyClusters) > 0 {
				errMessage = strings.Join(notReadyClusters, ";")
				return false, nil
			}
			errMessage = ""
			return true, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to wait for all managed clusters to be ready: %w - %s", err, errMessage)
	}
	log.Infof("all %d managed clusters are ready for migration", len(event.ManagedClusters))
	return nil
}

// initializing create the permission for the migration service account, and enable auto-approval for registration
func (s *MigrationTargetSyncer) initializing(ctx context.Context,
	event *migration.ManagedClusterMigrationToEvent,
) error {
	msaName := event.ManagedServiceAccountName
	msaNamespace := event.ManagedServiceAccountInstallNamespace
	if err := s.ensureClusterManagerAutoApproval(ctx, msaName, msaNamespace); err != nil {
		return err
	}
	if err := s.ensureSubjectAccessReviewRole(ctx, msaName); err != nil {
		return err
	}
	if err := s.ensureSubjectAccessReviewRoleBinding(ctx, msaName, msaNamespace); err != nil {
		return err
	}
	// bind migration sa with "open-cluster-management:managedcluster:bootstrap:agent-registration"
	if err := s.ensureRegistrationClusterRoleBinding(ctx, msaName, msaNamespace); err != nil {
		return err
	}

	return nil
}

func (s *MigrationTargetSyncer) deploying(ctx context.Context, evt *cloudevents.Event) error {
	// only the handle the current migration event, ignore the previous ones
	log.Debugf("get migration event: %v", evt.Type())

	payload := evt.Data()
	resourceEvent := &migration.SourceClusterMigrationResources{}
	if err := json.Unmarshal(payload, resourceEvent); err != nil {
		log.Errorf("failed to unmarshal cluster migration resources %v", err)
		return err
	}

	if s.currentMigrationId != resourceEvent.MigrationId {
		log.Infof("ignore the deploying event: expected migrationId %s, but got  %s", s.currentMigrationId,
			resourceEvent.MigrationId)
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.syncMigrationResources(ctx, resourceEvent)
	})
}

func (s *MigrationTargetSyncer) ensureNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, s.client, ns, func() error { return nil }); err != nil {
		log.Errorf("failed to create or update the namespace %s", ns.Name)
		return err
	}
	return nil
}

func (s *MigrationTargetSyncer) syncMigrationResources(ctx context.Context,
	migrationResources *migration.SourceClusterMigrationResources,
) error {
	log.Infof("started the deploying: %s", migrationResources.MigrationId)
	log.Debugf("migration resources %v", migrationResources)

	for _, mc := range migrationResources.ManagedClusters {
		if err := s.ensureNamespace(ctx, mc.Name); err != nil {
			return err
		}
		if _, err := controllerutil.CreateOrUpdate(ctx, s.client, &mc, func() error { return nil }); err != nil {
			log.Errorf("failed to create or update the managed cluster %s: %v", mc.Name, err)
			return err
		}
	}
	for _, config := range migrationResources.KlusterletAddonConfig {
		if _, err := controllerutil.CreateOrUpdate(ctx, s.client, &config, func() error { return nil }); err != nil {
			log.Errorf("failed to create or update the klusterlet addon config %s: %v", config.Name, err)
			return err
		}
	}

	log.Info("finished syncing migration resources")
	return nil
}

func (s *MigrationTargetSyncer) ensureClusterManagerAutoApproval(ctx context.Context,
	saName, saNamespace string,
) error {
	foundClusterManager := &operatorv1.ClusterManager{}
	if err := s.client.Get(ctx,
		types.NamespacedName{Name: "cluster-manager"}, foundClusterManager); err != nil {
		return err
	}

	clusterManager := foundClusterManager.DeepCopy()
	// check if the ManagedClusterAutoApproval feature is enabled and
	// the service account is added to the auto-approve list
	autoApproveUser := fmt.Sprintf("system:serviceaccount:%s:%s", saNamespace, saName)
	autoApproveFeatureEnabled := false
	autoApproveUserAdded := false
	clusterManagerChanged := false
	if clusterManager.Spec.RegistrationConfiguration != nil {
		registrationConfiguration := clusterManager.Spec.RegistrationConfiguration
		for i, featureGate := range registrationConfiguration.FeatureGates {
			if featureGate.Feature == "ManagedClusterAutoApproval" {
				autoApproveFeatureEnabled = true
				if featureGate.Mode == operatorv1.FeatureGateModeTypeEnable {
					break
				} else {
					registrationConfiguration.FeatureGates[i].Mode = operatorv1.FeatureGateModeTypeEnable
					clusterManagerChanged = true
					break
				}
			}
		}
		if !autoApproveFeatureEnabled {
			registrationConfiguration.FeatureGates = append(
				registrationConfiguration.FeatureGates,
				operatorv1.FeatureGate{
					Feature: "ManagedClusterAutoApproval",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				})
			clusterManagerChanged = true
		}
		for _, user := range registrationConfiguration.AutoApproveUsers {
			if user == autoApproveUser {
				autoApproveUserAdded = true
				break
			}
		}
		if !autoApproveUserAdded {
			registrationConfiguration.AutoApproveUsers = append(
				registrationConfiguration.AutoApproveUsers,
				autoApproveUser)
			clusterManagerChanged = true
		}
	} else {
		clusterManager.Spec.RegistrationConfiguration = &operatorv1.RegistrationHubConfiguration{
			FeatureGates: []operatorv1.FeatureGate{
				{
					Feature: "ManagedClusterAutoApproval",
					Mode:    operatorv1.FeatureGateModeTypeEnable,
				},
			},
			AutoApproveUsers: []string{
				autoApproveUser,
			},
		}
		clusterManagerChanged = true
	}

	// patch cluster-manager only if it has changed
	if clusterManagerChanged {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := s.client.Update(ctx, clusterManager); err != nil {
				return err
			}
			return nil
		}); err != nil {
			log.Errorw("failed to update clusterManager", "error", err)
		}
	}

	return nil
}

func (s *MigrationTargetSyncer) ensureSubjectAccessReviewRole(ctx context.Context, msaName string) error {
	// create or update clusterrole for the migration service account
	migrationClusterRoleName := getMigrationClusterRoleName(msaName)
	migrationClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: migrationClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}

	foundMigrationClusterRole := &rbacv1.ClusterRole{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: migrationClusterRoleName,
		}, foundMigrationClusterRole); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("creating migration clusterrole %s", foundMigrationClusterRole.GetName())
			log.Debugf("creating migration clusterrole %v", foundMigrationClusterRole)
			if err := s.client.Create(ctx, migrationClusterRole); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(migrationClusterRole, foundMigrationClusterRole) {
			log.Infof("updating migration clusterrole %s", migrationClusterRole.GetName())
			log.Debugf("updating migration clusterrole %v", migrationClusterRole)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := s.client.Update(ctx, migrationClusterRole); err != nil {
					return err
				}
				return nil
			}); err != nil {
				log.Errorw("failed to update migration ClusterRole", "error", err)
			}
		}
	}

	return nil
}

func (s *MigrationTargetSyncer) ensureRegistrationClusterRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	registrationClusterRoleName := "open-cluster-management:managedcluster:bootstrap:agent-registration"
	registrationClusterRoleBindingName := getAgentRegistrationClusterRoleBindingName(msaName)
	registrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrationClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      msaName,
				Namespace: msaNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     registrationClusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	foundRegistrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: registrationClusterRoleBindingName,
		}, foundRegistrationClusterRoleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("creating agent registration clusterrolebinding",
				"clusterrolebinding", registrationClusterRoleBindingName)
			if err := s.client.Create(ctx, registrationClusterRoleBinding); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(registrationClusterRoleBinding, foundRegistrationClusterRoleBinding) {
			log.Info("updating agent registration clusterrolebinding",
				"clusterrolebinding", registrationClusterRoleBindingName)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := s.client.Update(ctx, registrationClusterRoleBinding); err != nil {
					return err
				}
				return nil
			}); err != nil {
				log.Errorw("failed to update migration ClusterRoleBinding", "error", err)
			}
		}
	}

	return nil
}

func (s *MigrationTargetSyncer) ensureSubjectAccessReviewRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	migrationClusterRoleName := getMigrationClusterRoleName(msaName)
	sarMigrationClusterRoleBindingName := getSubjectAccessReviewClusterRoleBindingName(msaName)
	sarMigrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: sarMigrationClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      msaName,
				Namespace: msaNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     migrationClusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	foundSAMigrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: sarMigrationClusterRoleBindingName,
		}, foundSAMigrationClusterRoleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("creating subjectaccessreviews clusterrolebinding %s",
				foundSAMigrationClusterRoleBinding.GetName())
			log.Debugf("creating subjectaccessreviews clusterrolebinding %v",
				foundSAMigrationClusterRoleBinding)
			if err := s.client.Create(ctx, sarMigrationClusterRoleBinding); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !apiequality.Semantic.DeepDerivative(sarMigrationClusterRoleBinding, foundSAMigrationClusterRoleBinding) {
			log.Infof("updating subjectaccessreviews clusterrolebinding %v",
				sarMigrationClusterRoleBinding.GetName())
			log.Debugf("updating subjectaccessreviews clusterrolebinding %v",
				sarMigrationClusterRoleBinding)
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := s.client.Update(ctx, sarMigrationClusterRoleBinding); err != nil {
					return err
				}
				return nil
			}); err != nil {
				log.Errorw("failed to update subjectaccessreviews ClusterRoleBinding", "error", err)
			}

		}
	}

	return nil
}

func getMigrationClusterRoleName(managedServiceAccountName string) string {
	return fmt.Sprintf("multicluster-global-hub-migration:%s", managedServiceAccountName)
}

func getSubjectAccessReviewClusterRoleBindingName(managedServiceAccountName string) string {
	return fmt.Sprintf("%s-subjectaccessreviews-clusterrolebinding", managedServiceAccountName)
}

func getAgentRegistrationClusterRoleBindingName(managedServiceAccountName string) string {
	return fmt.Sprintf("agent-registration-clusterrolebinding:%s", managedServiceAccountName)
}

// rollbacking handles rollback operations for different stages on the target hub
func (s *MigrationTargetSyncer) rollbacking(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	log.Infof("performing rollback for stage: %s", migrationTargetHubEvent.RollbackStage)
	switch migrationTargetHubEvent.RollbackStage {
	case migrationv1alpha1.PhaseInitializing:
		return s.rollbackInitializing(ctx, migrationTargetHubEvent)
	case migrationv1alpha1.PhaseDeploying:
		return s.rollbackDeploying(ctx, migrationTargetHubEvent)
	case migrationv1alpha1.PhaseRegistering:
		return s.rollbackRegistering(ctx, migrationTargetHubEvent)
	default:
		return fmt.Errorf("no specific rollback action needed for stage: %s", migrationTargetHubEvent.RollbackStage)
	}
}

// rollbackInitializing handles rollback of initializing phase on target hub
func (s *MigrationTargetSyncer) rollbackInitializing(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	// For initializing rollback on target hub, mainly clean up RBAC resources
	// that were created for the managed service account

	if migrationTargetHubEvent.ManagedServiceAccountName == "" {
		log.Info("no managed service account name provided, skipping RBAC cleanup")
		return nil
	}

	return s.cleanupMigrationRBAC(ctx, migrationTargetHubEvent.ManagedServiceAccountName)
}

// rollbackDeploying handles rollback of deploying phase on target hub
// This is the main rollback operation that removes addonConfig and clusters
func (s *MigrationTargetSyncer) rollbackDeploying(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	log.Infof("rollback deploying stage for clusters: %v", migrationTargetHubEvent.ManagedClusters)

	// 1. Remove ManagedClusters from target hub
	for _, clusterName := range migrationTargetHubEvent.ManagedClusters {
		if err := s.removeManagedCluster(ctx, clusterName); err != nil {
			log.Errorf("failed to remove managed cluster %s: %v", clusterName, err)
			// Continue with other clusters even if one fails
		}
	}

	// 2. Remove KlusterletAddonConfigs from target hub
	for _, clusterName := range migrationTargetHubEvent.ManagedClusters {
		if err := s.removeKlusterletAddonConfig(ctx, clusterName); err != nil {
			log.Errorf("failed to remove klusterlet addon config %s: %v", clusterName, err)
			// Continue with other configs even if one fails
		}
	}

	// 3. Cleanup RBAC resources if managed service account name is provided
	if migrationTargetHubEvent.ManagedServiceAccountName != "" {
		if err := s.cleanupMigrationRBAC(ctx, migrationTargetHubEvent.ManagedServiceAccountName); err != nil {
			log.Errorf("failed to cleanup migration RBAC: %v", err)
		}
	}

	log.Info("completed deploying stage rollback")
	return nil
}

// rollbackRegistering handles rollback of registering phase on target hub
func (s *MigrationTargetSyncer) rollbackRegistering(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	log.Infof("rollback registering stage for clusters: %v", migrationTargetHubEvent.ManagedClusters)

	// For registering rollback, we need to do similar cleanup as deploying
	// since clusters might have been partially registered
	return s.rollbackDeploying(ctx, migrationTargetHubEvent)
}

// removeManagedCluster removes a ManagedCluster from the target hub
func (s *MigrationTargetSyncer) removeManagedCluster(ctx context.Context, clusterName string) error {
	managedCluster := &clusterv1.ManagedCluster{}
	err := s.client.Get(ctx, types.NamespacedName{Name: clusterName}, managedCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("managed cluster %s not found, already removed", clusterName)
			return nil
		}
		return fmt.Errorf("failed to get managed cluster %s: %w", clusterName, err)
	}

	if err := s.client.Delete(ctx, managedCluster); err != nil {
		return fmt.Errorf("failed to delete managed cluster %s: %w", clusterName, err)
	}

	log.Infof("successfully removed managed cluster: %s", clusterName)
	return nil
}

// removeKlusterletAddonConfig removes a KlusterletAddonConfig from the target hub
func (s *MigrationTargetSyncer) removeKlusterletAddonConfig(ctx context.Context, clusterName string) error {
	addonConfig := &addonv1.KlusterletAddonConfig{}
	err := s.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: clusterName}, addonConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("klusterlet addon config %s not found, already removed", clusterName)
			return nil
		}
		return fmt.Errorf("failed to get klusterlet addon config %s: %w", clusterName, err)
	}

	if err := s.client.Delete(ctx, addonConfig); err != nil {
		return fmt.Errorf("failed to delete klusterlet addon config %s: %w", clusterName, err)
	}

	log.Infof("successfully removed klusterlet addon config: %s", clusterName)
	return nil
}

// cleanupMigrationRBAC removes RBAC resources created for the managed service account
func (s *MigrationTargetSyncer) cleanupMigrationRBAC(ctx context.Context, managedServiceAccountName string) error {
	// Remove ClusterRole for SubjectAccessReview
	clusterRoleName := getMigrationClusterRoleName(managedServiceAccountName)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
	}
	if err := s.deleteResourceIfExists(ctx, clusterRole); err != nil {
		log.Errorf("failed to delete cluster role %s: %v", clusterRoleName, err)
		return err
	}

	// Remove ClusterRoleBinding for SubjectAccessReview
	sarClusterRoleBindingName := getSubjectAccessReviewClusterRoleBindingName(managedServiceAccountName)
	sarClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: sarClusterRoleBindingName,
		},
	}
	if err := s.deleteResourceIfExists(ctx, sarClusterRoleBinding); err != nil {
		log.Errorf("failed to delete subjectaccessreviews cluster role binding %s: %v", sarClusterRoleBindingName, err)
		return err
	}

	// Remove ClusterRoleBinding for Agent Registration
	registrationClusterRoleBindingName := getAgentRegistrationClusterRoleBindingName(managedServiceAccountName)
	registrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrationClusterRoleBindingName,
		},
	}
	if err := s.deleteResourceIfExists(ctx, registrationClusterRoleBinding); err != nil {
		log.Errorf("failed to delete agent registration cluster role binding %s: %v", registrationClusterRoleBindingName, err)
		return err
	}

	return nil
}

// checkClusterManifestWork checks if a cluster's ManifestWork is ready
func (s *MigrationTargetSyncer) checkClusterManifestWork(ctx context.Context, clusterName string) error {
	// ManifestWork name pattern: <cluster-name>-klusterlet
	workName := fmt.Sprintf("%s%s", clusterName, KlusterletManifestWorkSuffix)
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName,
			Name:      workName,
		},
	}

	if err := s.client.Get(ctx, client.ObjectKeyFromObject(work), work); err != nil {
		return fmt.Errorf("failed to get manifestwork %s: %w", workName, err)
	}

	if !meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkApplied) {
		return fmt.Errorf("manifestwork %s is not applied", workName)
	}

	return nil
}

// deleteResourceIfExists deletes a resource if it exists, ignoring NotFound errors
func (s *MigrationTargetSyncer) deleteResourceIfExists(ctx context.Context, obj client.Object) error {
	if obj == nil {
		return nil
	}

	if err := s.client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debugf("resource %s/%s already deleted", obj.GetNamespace(), obj.GetName())
			return nil
		}
		return fmt.Errorf("failed to get resource: %w", err)
	}

	log.Infof("deleting resource %s/%s", obj.GetNamespace(), obj.GetName())
	if err := s.client.Delete(ctx, obj); err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	return nil
}
