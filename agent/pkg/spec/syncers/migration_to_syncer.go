// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"fmt"
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

type migrationTargetSyncer struct {
	client             client.Client
	transportClient    transport.TransportClient
	transportConfig    *transport.TransportInternalConfig
	bundleVersion      *eventversion.Version
	currentMigrationId string
}

func NewMigrationTargetSyncer(client client.Client,
	transportClient transport.TransportClient, transportConfig *transport.TransportInternalConfig,
) *migrationTargetSyncer {
	return &migrationTargetSyncer{
		client:          client,
		transportClient: transportClient,
		transportConfig: transportConfig,
		bundleVersion:   eventversion.NewVersion(),
	}
}

func (s *migrationTargetSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	log.Infof("received migration event from %s", evt.Source())
	if evt.Source() == constants.CloudEventGlobalHubClusterName {
		managedClusterMigrationToEvent := &migration.ManagedClusterMigrationToEvent{}
		if err := json.Unmarshal(evt.Data(), managedClusterMigrationToEvent); err != nil {
			return fmt.Errorf("failed to unmarshal payload %v", err)
		}
		log.Debugf("received cloudevent %v", string(evt.Data()))
		if managedClusterMigrationToEvent.MigrationId == "" {
			return fmt.Errorf("must set the migrationId: %v", evt)
		}

		log.Infof("target hub handle the migration: %s", managedClusterMigrationToEvent.MigrationId)

		if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseInitializing {
			s.currentMigrationId = managedClusterMigrationToEvent.MigrationId
			// reset the bundle version for each new migration
			s.bundleVersion.Reset()
			if err := s.initializing(ctx, managedClusterMigrationToEvent); err != nil {
				log.Errorf("failed to initialize the migration resources %v", err)
				return err
			}
			log.Info("finished the initializing")
		}

		if s.currentMigrationId != managedClusterMigrationToEvent.MigrationId {
			log.Infof("ignore the migration event: expected migrationId %s, but got  %s",
				s.currentMigrationId, managedClusterMigrationToEvent.MigrationId)
			return nil
		}

		if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseRegistering {
			go func() {
				log.Info("registering managed cluster migration")
				reportErrMessage := ""
				err := wait.PollUntilContextTimeout(ctx, 5*time.Second, registeringTimeout, true,
					func(context.Context) (done bool, err error) {
						if e := s.registering(ctx, managedClusterMigrationToEvent); e != nil {
							log.Infof("waiting the migrating clusters are available: %v", e)
							reportErrMessage = e.Error()
							return false, nil
						}
						log.Info("finished registering clusters")
						reportErrMessage = ""
						return true, nil
					},
				)
				// the reportErrMessage record the detailed information why the registering failed,
				// if registered successfully, remove the record error message during the registering
				if err != nil {
					reportErrMessage = fmt.Sprintf("registering %s - %s", err.Error(), reportErrMessage)
				}

				err = ReportMigrationStatus(
					cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
					s.transportClient,
					&migration.ManagedClusterMigrationBundle{
						MigrationId: managedClusterMigrationToEvent.MigrationId,
						Stage:       migrationv1alpha1.ConditionTypeRegistered,
						ErrMessage:  reportErrMessage,
					}, s.bundleVersion)
				if err != nil {
					log.Errorf("failed to report the registering migration event due to %v", err)
				} else {
					log.Info("report registering process successfully")
				}
			}()
		}

		if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseCleaning ||
			managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseFailed {
			log.Infof("cleaning up managed cluster migration: %s", managedClusterMigrationToEvent.Stage)

			if err := s.cleaning(ctx, managedClusterMigrationToEvent); err != nil {
				log.Errorf("failed to clean up the migration resources %v", err)
				return nil
			}
			log.Info("finished the cleaning up resources")
		}

		if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseRollbacking {
			log.Infof("rolling back managed cluster migration, rollback stage: %s", managedClusterMigrationToEvent.RollbackStage)

			if err := s.rollbacking(ctx, managedClusterMigrationToEvent); err != nil {
				log.Errorf("failed to rollback the migration resources %v", err)
				return err
			}
			log.Info("finished the rollback")
		}
	} else {
		if err := s.deploying(ctx, evt); err != nil {
			log.Errorf("failed to start migration consumer: %v", err)
			return err
		}
	}

	return nil
}

func (s *migrationTargetSyncer) cleaning(ctx context.Context,
	evt *migration.ManagedClusterMigrationToEvent,
) error {
	msaName := evt.ManagedServiceAccountName

	// delete the subjectaccessreviews creation role and roleBinding
	migrationClusterRoleName := getMigrationClusterRoleName(msaName)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: migrationClusterRoleName},
	}
	if err := s.client.Get(ctx,
		client.ObjectKeyFromObject(clusterRole), clusterRole); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		if err = s.client.Delete(ctx, clusterRole); err != nil {
			return err
		}
	}

	sarMigrationClusterRoleBindingName := fmt.Sprintf("%s-subjectaccessreviews-clusterrolebinding", msaName)
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: sarMigrationClusterRoleBindingName,
		},
	}
	if err := s.client.Get(ctx,
		client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		if err = s.client.Delete(ctx, clusterRoleBinding); err != nil {
			return err
		}
	}

	// report the cleaned up confirmation
	err := ReportMigrationStatus(
		cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: evt.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeCleaned,
		}, s.bundleVersion)
	if err != nil {
		return err
	}
	return nil
}

// registering watches the migrated managed clusters
func (s *migrationTargetSyncer) registering(ctx context.Context,
	evt *migration.ManagedClusterMigrationToEvent,
) error {
	if len(evt.ManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters found in migration event: %s", evt.MigrationId)
	}
	// notAvailableManagedClusters = notAvailableManagedClusters[:0]
	notAvailableManagedClusters := []string{}
	for _, cluster := range evt.ManagedClusters {
		// not support hosted managed hub, the hosted klusterletManifestWork name is <cluster-name>-hosted-klusterlet
		klusterletManifestWorkName := fmt.Sprintf("%s%s", cluster, KlusterletManifestWorkSuffix)
		work := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster,
				Name:      klusterletManifestWorkName,
			},
		}

		if err := s.client.Get(ctx, client.ObjectKeyFromObject(work), work); err != nil {
			log.Infof("failed to get manifestwork %s: %v", work.Name, err)
			notAvailableManagedClusters = append(notAvailableManagedClusters, cluster)
			continue
		}

		if !meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkApplied) {
			log.Infof("manifestwork %s is not applied", work.Name)
			notAvailableManagedClusters = append(notAvailableManagedClusters, cluster)
			continue
		}
	}

	if len(notAvailableManagedClusters) > 0 {
		return fmt.Errorf("manifestwork(*-klusterlet) are not applied in these clusters: %v", notAvailableManagedClusters)
	}
	return nil
}

// initializing create the permission for the migration service account, and enable auto-approval for registration
func (s *migrationTargetSyncer) initializing(ctx context.Context,
	evt *migration.ManagedClusterMigrationToEvent,
) error {
	msaName := evt.ManagedServiceAccountName
	msaNamespace := evt.ManagedServiceAccountInstallNamespace
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

	return ReportMigrationStatus(
		cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: evt.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeInitialized,
		},
		s.bundleVersion)
}

func (s *migrationTargetSyncer) deploying(ctx context.Context, evt *cloudevents.Event) error {
	// only the handle the current migration event, ignore the previous ones
	log.Debugf("get migration event: %v", evt.Type())

	payload := evt.Data()
	migrationResources := &migration.SourceClusterMigrationResources{}
	if err := json.Unmarshal(payload, migrationResources); err != nil {
		log.Errorf("failed to unmarshal cluster migration resources %v", err)
		return err
	}

	migrationId := migrationResources.MigrationId
	if s.currentMigrationId != migrationId {
		log.Infof("ignore the deploying event: expected migrationId %s, but got  %s", s.currentMigrationId, migrationId)
		return nil
	}

	var err error

	defer func() {
		deployingReportBundle := &migration.ManagedClusterMigrationBundle{
			MigrationId: migrationResources.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeDeployed,
		}
		if err != nil {
			deployingReportBundle.ErrMessage = err.Error()
		}

		// report the deploying confirmation
		if err := ReportMigrationStatus(
			cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
			s.transportClient, deployingReportBundle, s.bundleVersion); err != nil {
			log.Errorf("failed to report the deploying migration event due to %v", err)
			return
		}
		log.Infof("finished the deploying %s", migrationResources.MigrationId)
	}()

	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.syncMigrationResources(ctx, migrationResources)
	}); err != nil {
		log.Errorw("failed to deploying", "type", evt.Type(), "error", err)
	}
	return nil
}

func (s *migrationTargetSyncer) ensureNamespace(ctx context.Context, namespace string) error {
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

func (s *migrationTargetSyncer) syncMigrationResources(ctx context.Context,
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

	for _, configMap := range migrationResources.ConfigMaps {
		if err := s.ensureNamespace(ctx, configMap.Namespace); err != nil {
			return err
		}

		if err := s.client.Create(ctx, configMap); err != nil {
			log.Errorf("failed to create configMap(%s): %v", configMap.Name, err)
			return err
		}
	}

	for _, secret := range migrationResources.Secrets {
		if err := s.ensureNamespace(ctx, secret.Namespace); err != nil {
			return err
		}
		if err := s.client.Create(ctx, secret); err != nil {
			log.Errorf("failed to create the secret %s: &v", secret.Name, err)
			return err
		}
	}

	log.Info("finished syncing migration resources")
	return nil
}

func (s *migrationTargetSyncer) ensureClusterManagerAutoApproval(ctx context.Context,
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

func (s *migrationTargetSyncer) ensureSubjectAccessReviewRole(ctx context.Context, msaName string) error {
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

func (s *migrationTargetSyncer) ensureRegistrationClusterRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	registrationClusterRoleName := "open-cluster-management:managedcluster:bootstrap:agent-registration"
	registrationClusterRoleBindingName := fmt.Sprintf("agent-registration-clusterrolebinding:%s", msaName)
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

func (s *migrationTargetSyncer) ensureSubjectAccessReviewRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	migrationClusterRoleName := getMigrationClusterRoleName(msaName)
	sarMigrationClusterRoleBindingName := fmt.Sprintf("%s-subjectaccessreviews-clusterrolebinding", msaName)
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

// rollbacking handles rollback operations for different stages on the target hub
func (s *migrationTargetSyncer) rollbacking(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	log.Infof("performing rollback for stage: %s", migrationTargetHubEvent.RollbackStage)

	var err error
	var reportMessage string

	switch migrationTargetHubEvent.RollbackStage {
	case migrationv1alpha1.PhaseInitializing:
		err = s.rollbackInitializing(ctx, migrationTargetHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Initializing rollback on target hub completed with issues: %v", err)
		} else {
			reportMessage = "RBAC resources successfully cleaned up on target hub"
		}
	case migrationv1alpha1.PhaseDeploying:
		err = s.rollbackDeploying(ctx, migrationTargetHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Deploying rollback on target hub completed with issues: %v", err)
		} else {
			reportMessage = "ManagedClusters and KlusterletAddonConfigs successfully removed from target hub"
		}
	case migrationv1alpha1.PhaseRegistering:
		err = s.rollbackRegistering(ctx, migrationTargetHubEvent)
		if err != nil {
			reportMessage = fmt.Sprintf("Registering rollback on target hub completed with issues: %v", err)
		} else {
			reportMessage = "Resources successfully cleaned up from target hub"
		}
	default:
		log.Infof("no specific rollback action needed for stage: %s", migrationTargetHubEvent.RollbackStage)
		reportMessage = fmt.Sprintf("No specific rollback action needed for stage: %s", migrationTargetHubEvent.RollbackStage)
	}

	// Report rollback status to global hub, including detailed message
	reportErr := ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: migrationTargetHubEvent.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeRolledBack,
			ErrMessage:  reportMessage, // Use detailed message instead of just error
		},
		s.bundleVersion)

	if reportErr != nil {
		log.Errorf("failed to report rollback status: %v", reportErr)
	}

	// Return the original error if any
	return err
}

// rollbackInitializing handles rollback of initializing phase on target hub
func (s *migrationTargetSyncer) rollbackInitializing(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
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
func (s *migrationTargetSyncer) rollbackDeploying(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
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
func (s *migrationTargetSyncer) rollbackRegistering(ctx context.Context, migrationTargetHubEvent *migration.ManagedClusterMigrationToEvent) error {
	log.Infof("rollback registering stage for clusters: %v", migrationTargetHubEvent.ManagedClusters)

	// For registering rollback, we need to do similar cleanup as deploying
	// since clusters might have been partially registered
	return s.rollbackDeploying(ctx, migrationTargetHubEvent)
}

// removeManagedCluster removes a ManagedCluster from the target hub
func (s *migrationTargetSyncer) removeManagedCluster(ctx context.Context, clusterName string) error {
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
func (s *migrationTargetSyncer) removeKlusterletAddonConfig(ctx context.Context, clusterName string) error {
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
func (s *migrationTargetSyncer) cleanupMigrationRBAC(ctx context.Context, managedServiceAccountName string) error {
	clusterRoleName := getMigrationClusterRoleName(managedServiceAccountName)
	clusterRoleBindingName := clusterRoleName

	// Remove ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := s.client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName}, clusterRoleBinding)
	if err == nil {
		if err := s.client.Delete(ctx, clusterRoleBinding); err != nil {
			log.Errorf("failed to delete cluster role binding %s: %v", clusterRoleBindingName, err)
		} else {
			log.Infof("successfully removed cluster role binding: %s", clusterRoleBindingName)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Errorf("failed to get cluster role binding %s: %v", clusterRoleBindingName, err)
	}

	// Remove ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	err = s.client.Get(ctx, types.NamespacedName{Name: clusterRoleName}, clusterRole)
	if err == nil {
		if err := s.client.Delete(ctx, clusterRole); err != nil {
			log.Errorf("failed to delete cluster role %s: %v", clusterRoleName, err)
		} else {
			log.Infof("successfully removed cluster role: %s", clusterRoleName)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Errorf("failed to get cluster role %s: %v", clusterRoleName, err)
	}

	return nil
}
