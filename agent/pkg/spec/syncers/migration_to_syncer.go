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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

	if evt.Source() != constants.CloudEventGlobalHubClusterName {
		return s.deploying(ctx, evt)
	}

	managedClusterMigrationToEvent := &migration.ManagedClusterMigrationToEvent{}
	if err := json.Unmarshal(evt.Data(), managedClusterMigrationToEvent); err != nil {
		return fmt.Errorf("failed to unmarshal payload %v", err)
	}
	log.Debugf("received cloudevent %v", string(evt.Data()))
	if managedClusterMigrationToEvent.MigrationId == "" {
		return fmt.Errorf("migrationId is required but not provided in event")
	}

	var err error

	defer func() {
		if s.currentMigrationId != managedClusterMigrationToEvent.MigrationId {
			return
		}
		errMessage := ""
		if err != nil {
			errMessage = err.Error()
		}
		err := ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
			s.transportClient,
			&migration.ManagedClusterMigrationBundle{
				MigrationId: managedClusterMigrationToEvent.MigrationId,
				Stage:       managedClusterMigrationToEvent.Stage,
				ErrMessage:  errMessage,
			},
			s.bundleVersion)
		if err != nil {
			log.Errorf("failed to send the %s confirmation: %v", managedClusterMigrationToEvent.Stage, err)
		}
	}()

	if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseInitializing {
		s.currentMigrationId = managedClusterMigrationToEvent.MigrationId
		// reset the bundle version for each new migration
		s.bundleVersion.Reset()
		log.Infof("migration initializing started: migrationId=%s, clusters=%v, msaName=%s",
			managedClusterMigrationToEvent.MigrationId, managedClusterMigrationToEvent.ManagedClusters,
			managedClusterMigrationToEvent.ManagedServiceAccountName)
		if err := s.initializing(ctx, managedClusterMigrationToEvent); err != nil {
			log.Errorf("migration initializing failed: migrationId=%s, error=%v", managedClusterMigrationToEvent.MigrationId,
				err)
			return err
		}
		log.Infof("migration initializing completed: migrationId=%s", managedClusterMigrationToEvent.MigrationId)
	}

	if s.currentMigrationId != managedClusterMigrationToEvent.MigrationId {
		log.Infof("ignore the migration event: expected migrationId %s, but got  %s",
			s.currentMigrationId, managedClusterMigrationToEvent.MigrationId)
		return nil
	}

	if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseRegistering {
		log.Infof("migration registering is starting: %s", managedClusterMigrationToEvent.MigrationId)
		if err := s.registering(ctx, managedClusterMigrationToEvent); err != nil {
			log.Errorf("failed to registering clusters: %v", err)
			return err
		}
		log.Infof("migration registering is finished: %s", managedClusterMigrationToEvent.MigrationId)
	}

	if managedClusterMigrationToEvent.Stage == migrationv1alpha1.PhaseCleaning {
		log.Infof("migration cleaning is starting: %s", managedClusterMigrationToEvent.MigrationId)
		if err := s.cleaning(ctx, managedClusterMigrationToEvent); err != nil {
			log.Errorf("failed to clean up the migration resources %v", err)
			return err
		}
		log.Infof("migration cleaning is finished: %s", managedClusterMigrationToEvent.MigrationId)
	}

	return nil
}

func (s *MigrationTargetSyncer) cleaning(ctx context.Context,
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

	// delete the agent registration cluster role binding
	registrationClusterRoleBindingName := fmt.Sprintf("agent-registration-clusterrolebinding:%s", msaName)
	registrationClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: registrationClusterRoleBindingName,
		},
	}
	if err := s.client.Get(ctx,
		client.ObjectKeyFromObject(registrationClusterRoleBinding), registrationClusterRoleBinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		if err = s.client.Delete(ctx, registrationClusterRoleBinding); err != nil {
			return err
		}
	}
	return nil
}

// registering watches the migrated managed clusters
func (s *MigrationTargetSyncer) registering(ctx context.Context,
	evt *migration.ManagedClusterMigrationToEvent,
) error {
	if len(evt.ManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters found in migration event: %s", evt.MigrationId)
	}
	errMessage := ""
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, registeringTimeout, true,
		func(context.Context) (done bool, err error) {
			notAvailableClusters := []string{}
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
					notAvailableClusters = append(notAvailableClusters, cluster)
					continue
				}

				if !meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkApplied) {
					log.Infof("manifestwork %s is not applied", work.Name)
					notAvailableClusters = append(notAvailableClusters, cluster)
					continue
				}
			}

			if len(notAvailableClusters) > 0 {
				errMessage = fmt.Sprintf("waiting the manifestworks(*-klusterlet) to be applied in clusters: %v",
					notAvailableClusters)
				log.Infof(errMessage)
				return false, nil
			}
			errMessage = ""
			return true, nil
		},
	)
	if err != nil {
		return fmt.Errorf("%w - %s", err, errMessage)
	}
	return nil
}

// initializing create the permission for the migration service account, and enable auto-approval for registration
func (s *MigrationTargetSyncer) initializing(ctx context.Context,
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

	return nil
}

func (s *MigrationTargetSyncer) deploying(ctx context.Context, evt *cloudevents.Event) error {
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

	log.Infof("migration deploying is starting: %s", migrationId)

	deployingReportBundle := &migration.ManagedClusterMigrationBundle{
		MigrationId: migrationResources.MigrationId,
		Stage:       migrationv1alpha1.PhaseDeploying,
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.syncMigrationResources(ctx, migrationResources)
	})
	if err != nil {
		log.Errorw("failed to deploying", "type", evt.Type(), "error", err)
		deployingReportBundle.ErrMessage = err.Error()
	}

	// report the deploying confirmation
	if err := ReportMigrationStatus(cecontext.WithTopic(ctx, s.transportConfig.KafkaCredential.StatusTopic),
		s.transportClient, deployingReportBundle, s.bundleVersion); err != nil {
		log.Errorf("failed to report the deploying migration event due to %v", err)
		return err
	}

	log.Infof("migration deploying is finished: %s", migrationId)
	return nil
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
	if msaName == "" || msaNamespace == "" {
		return fmt.Errorf("managed service account name and namespace are required")
	}
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

func (s *MigrationTargetSyncer) ensureSubjectAccessReviewRoleBinding(ctx context.Context,
	msaName, msaNamespace string,
) error {
	if msaName == "" || msaNamespace == "" {
		return fmt.Errorf("managed service account name and namespace are required")
	}
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
