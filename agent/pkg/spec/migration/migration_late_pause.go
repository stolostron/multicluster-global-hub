// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const migrationAppliedWorkAnnotation = "global-hub.open-cluster-management.io/migration-applied-work"

// The gate must read API state rather than lagging manager caches.
func (s *MigrationSourceSyncer) latePauseClient() (client.Client, error) {
	if s.restConfig == nil {
		return s.client, nil
	}
	return client.New(s.restConfig, client.Options{Scheme: s.client.Scheme(), Mapper: s.client.RESTMapper()})
}

// appliedMigrationWork verifies controller-rendered payload, not just Applied=True
// on an unrelated generation. It never changes a ManifestWork or its strategy.
func appliedMigrationWork(work *workv1.ManifestWork, secret *corev1.Secret, migrationID string, recorded string) (string, error) {
	if migrationID == "" || len(secret.Data["kubeconfig"]) == 0 || !work.DeletionTimestamp.IsZero() {
		return "", fmt.Errorf("missing migration credentials or deleting source work")
	}
	targetPresent, registrationPresent := false, false
	for _, manifest := range work.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(manifest.Raw, &obj.Object); err != nil {
			return "", fmt.Errorf("invalid source work manifest")
		}
		if obj.GetAPIVersion() == "v1" && obj.GetKind() == "Secret" && obj.GetName() == secret.Name && obj.GetNamespace() == "open-cluster-management-agent" {
			data, _, _ := unstructured.NestedString(obj.Object, "data", "kubeconfig")
			targetPresent = data == base64.StdEncoding.EncodeToString(secret.Data["kubeconfig"])
		}
		if obj.GetAPIVersion() != "operator.open-cluster-management.io/v1" || obj.GetKind() != "Klusterlet" {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "registrationConfiguration", "bootstrapKubeConfigs", "localSecretsConfig", "kubeConfigSecrets")
		typ, _, _ := unstructured.NestedString(obj.Object, "spec", "registrationConfiguration", "bootstrapKubeConfigs", "type")
		gates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "registrationConfiguration", "featureGates")
		enabled, targetRef := false, false
		for _, ref := range refs {
			if r, ok := ref.(map[string]interface{}); ok && r["name"] == secret.Name {
				targetRef = true
			}
		}
		for _, gate := range gates {
			if g, ok := gate.(map[string]interface{}); ok && g["feature"] == "MultipleHubs" && g["mode"] == "Enable" {
				enabled = true
			}
		}
		registrationPresent = registrationPresent || (enabled && targetRef && typ == "LocalSecrets")
	}
	if !targetPresent || !registrationPresent {
		return "", fmt.Errorf("source work lacks target bootstrap credentials or MultipleHubs registration")
	}
	// Bind retries to the exact payload and work UID. ReadOnly changes only
	// ManifestConfigs, so they must not invalidate the successful apply record.
	payload, err := json.Marshal(work.Spec.Workload)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%s %s %x", migrationID, work.UID, sha256.Sum256(payload))
	if recorded == digest {
		return digest, nil
	}
	for _, cfg := range work.Spec.ManifestConfigs {
		if cfg.UpdateStrategy != nil && cfg.UpdateStrategy.Type == workv1.UpdateStrategyTypeReadOnly {
			return "", fmt.Errorf("source work became ReadOnly before its migration payload was verified")
		}
	}
	applied := meta.FindStatusCondition(work.Status.Conditions, workv1.WorkApplied)
	if work.Generation < 1 || applied == nil || applied.Status != metav1.ConditionTrue || applied.ObservedGeneration != work.Generation {
		return "", fmt.Errorf("source work has not applied its current migration generation")
	}
	return digest, nil
}

func checkLatePauseWork(ctx context.Context, c client.Client, cluster string, event *migration.MigrationSourceBundle, secret *corev1.Secret) (string, error) {
	mc := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: cluster}, mc); err != nil {
		return "", err
	}
	work := &workv1.ManifestWork{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: cluster, Name: cluster + "-klusterlet"}, work); err != nil {
		return "", err
	}
	return appliedMigrationWork(work, secret, event.MigrationId, mc.Annotations[migrationAppliedWorkAnnotation])
}

// pauseAfterMigrationApplied leaves admission enabled for the whole batch until
// every payload has applied. A failed pause/cutover is safe to retry after restart.
func (s *MigrationSourceSyncer) pauseAfterMigrationApplied(ctx context.Context, event *migration.MigrationSourceBundle) error {
	c, err := s.latePauseClient()
	if err != nil {
		return err
	}
	budget := 30 * time.Second
	if expiry := expireTimeFromContext(ctx); !expiry.IsZero() {
		budget = time.Until(expiry) - 20*time.Second
	}
	if budget <= 0 {
		return fmt.Errorf("insufficient time to verify migration work before cutover")
	}
	secret := &corev1.Secret{}
	err = wait.PollUntilContextTimeout(ctx, time.Second, budget, true, func(ctx context.Context) (bool, error) {
		if err := c.Get(ctx, client.ObjectKey{Namespace: "multicluster-engine", Name: bootstrapSecretNamePrefix + event.ToHub}, secret); err != nil {
			return false, err
		}
		for _, cluster := range event.ManagedClusters {
			if _, err := checkLatePauseWork(ctx, c, cluster, event, secret); err != nil {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("migration source work apply gate failed; source admission left unchanged: %w", err)
	}
	for _, cluster := range event.ManagedClusters {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			mc := &clusterv1.ManagedCluster{}
			if err := c.Get(ctx, client.ObjectKey{Name: cluster}, mc); err != nil {
				return err
			}
			digest, err := checkLatePauseWork(ctx, c, cluster, event, secret)
			if err != nil {
				return err
			}
			if mc.Annotations == nil {
				mc.Annotations = map[string]string{}
			}
			mc.Annotations[migrationAppliedWorkAnnotation] = digest
			mc.Annotations[apiconstants.DisableAutoImportAnnotation] = ""
			return c.Update(ctx, mc)
		}); err != nil {
			return err
		}
	}
	// Detect payload replacement during the pause before allowing cutover.
	for _, cluster := range event.ManagedClusters {
		if _, err := checkLatePauseWork(ctx, c, cluster, event, secret); err != nil {
			return err
		}
	}
	return nil
}
