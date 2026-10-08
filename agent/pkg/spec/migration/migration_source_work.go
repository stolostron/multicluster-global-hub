// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
)

const (
	sourceWorkBaselineAnnotation  = "global-hub.open-cluster-management.io/migration-work-baseline"
	spokeAgentNamespace           = "open-cluster-management-agent"
	sourceBootstrapSecretName     = "bootstrap-hub-kubeconfig"
	currentHubBootstrapSecretName = sourceBootstrapSecretName + "-current-hub"
	maxWorkBaselineSize           = 16 * 1024
)

// Only non-secret registration fields are saved, on the source work itself, so
// rollback survives an agent restart. Never store manifests or kubeconfig bytes
// in annotations. Absent fields remain absent when restored.
type sourceWorkBaseline struct {
	MigrationID     string          `json:"migrationID"`
	Target          string          `json:"target"`
	Klusterlet      string          `json:"klusterlet"`
	Bootstrap       json.RawMessage `json:"bootstrap,omitempty"`
	FeatureGates    json.RawMessage `json:"featureGates,omitempty"`
	Restoring       bool            `json:"restoring,omitempty"`
	AddedCurrentHub bool            `json:"addedCurrentHub,omitempty"`
}

// The manager client may serve reads from an informer cache. Use direct API
// reads for the generation gate and fresh conflict-retry reads, not a possibly
// stale Applied status. Tests without a REST config use their supplied client.
func (s *MigrationSourceSyncer) migrationSourceClient() (client.Client, error) {
	if s.restConfig == nil {
		return s.client, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sourceAPIClient == nil {
		apiClient, err := client.New(s.restConfig, client.Options{Scheme: s.client.Scheme(), Mapper: s.client.RESTMapper()})
		if err != nil {
			return nil, fmt.Errorf("cannot initialize uncached source-work client")
		}
		s.sourceAPIClient = apiClient
	}
	return s.sourceAPIClient, nil
}

func sourceWorkKey(cluster string) client.ObjectKey {
	return client.ObjectKey{Namespace: cluster, Name: cluster + "-klusterlet"}
}

func decodeSourceWork(work *workv1.ManifestWork) ([]*unstructured.Unstructured, int, error) {
	objects := make([]*unstructured.Unstructured, len(work.Spec.Workload.Manifests))
	klusterletIndex := -1
	identities := map[string]bool{}
	for i, manifest := range work.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(manifest.Raw, &obj.Object); err != nil || obj.Object == nil {
			return nil, -1, fmt.Errorf("invalid JSON in source work manifest %d", i)
		}
		identity := obj.GetAPIVersion() + "/" + obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
		if identities[identity] {
			return nil, -1, fmt.Errorf("duplicate resource identity in source work manifest %d", i)
		}
		identities[identity] = true
		objects[i] = obj
		if obj.GetKind() != "Klusterlet" {
			continue
		}
		if obj.GetAPIVersion() != "operator.open-cluster-management.io/v1" ||
			obj.GetNamespace() != "" || obj.GetName() == "" || klusterletIndex != -1 {
			return nil, -1, fmt.Errorf("missing or ambiguous supported Klusterlet in source work")
		}
		klusterletIndex = i
	}
	if klusterletIndex == -1 {
		return nil, -1, fmt.Errorf("source work has no Klusterlet")
	}
	return objects, klusterletIndex, nil
}

func secretManifestIndex(objects []*unstructured.Unstructured, name string) int {
	for i, obj := range objects {
		if obj.GetAPIVersion() == "v1" && obj.GetKind() == "Secret" &&
			obj.GetName() == name && obj.GetNamespace() == spokeAgentNamespace {
			return i
		}
	}
	return -1
}

func workResourceWritable(work *workv1.ManifestWork, group, resource, namespace, name string) error {
	for _, config := range work.Spec.ManifestConfigs {
		id := config.ResourceIdentifier
		// Some import-controller versions leave resource empty for work-agent
		// GVK inference. Those strategies still apply to the identified object.
		if id.Group != group || (id.Resource != "" && id.Resource != resource) || id.Namespace != namespace || id.Name != name {
			continue
		}
		if config.UpdateStrategy != nil &&
			(config.UpdateStrategy.Type == workv1.UpdateStrategyTypeCreateOnly ||
				config.UpdateStrategy.Type == workv1.UpdateStrategyTypeReadOnly) {
			return fmt.Errorf("source work %s/%s cannot update %s %s: %s strategy",
				work.Namespace, work.Name, resource, name, config.UpdateStrategy.Type)
		}
		// Completion rules can stop subsequent reconciliation even for an Update
		// strategy. Reject these rather than treating an old Applied as sufficient.
		for _, rule := range config.ConditionRules {
			if rule.Condition == "Complete" {
				return fmt.Errorf("source work has a completion rule on %s %s", resource, name)
			}
		}
	}
	return nil
}

func sourceSecretHasKubeconfig(obj *unstructured.Unstructured) bool {
	data, _, err := unstructured.NestedString(obj.Object, "data", "kubeconfig")
	if err != nil || data == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	return err == nil && len(decoded) > 0
}

// prepareSourceWork mutates only the migration-owned Secret and the two
// registration fields required for MultipleHubs, preserving all other raw
// manifests and work options. It is also used on a copy to validate the gate.
func prepareSourceWork(work *workv1.ManifestWork, event *migration.MigrationSourceBundle,
	secret *corev1.Secret,
) error {
	if event.MigrationId == "" || event.ToHub == "" || secret == nil ||
		secret.Name != bootstrapSecretNamePrefix+event.ToHub || len(secret.Data["kubeconfig"]) == 0 {
		return fmt.Errorf("migration requires a named target bootstrap Secret with nonempty kubeconfig")
	}
	objects, ki, err := decodeSourceWork(work)
	if err != nil {
		return err
	}
	klusterlet := objects[ki]
	mode, _, err := unstructured.NestedString(klusterlet.Object, "spec", "deployOption", "mode")
	if err != nil || (mode != "" && mode != "Default" && mode != "Singleton") {
		return fmt.Errorf("source work has unsupported Klusterlet install mode")
	}
	namespace, _, err := unstructured.NestedString(klusterlet.Object, "spec", "namespace")
	if err != nil || namespace != spokeAgentNamespace {
		return fmt.Errorf("source work has unsupported Klusterlet namespace")
	}
	if err := workResourceWritable(work, "operator.open-cluster-management.io", "klusterlets", "", klusterlet.GetName()); err != nil {
		return err
	}
	if err := workResourceWritable(work, "", "secrets", spokeAgentNamespace, secret.Name); err != nil {
		return err
	}
	// Keep the default bootstrap path untouched. MultipleHubs may replace its
	// spoke-side contents when rebootstrap selects a hub, so the fallback must
	// use a separate current-hub Secret, as in the import controller renderer.
	si := secretManifestIndex(objects, sourceBootstrapSecretName)
	if si == -1 || !sourceSecretHasKubeconfig(objects[si]) {
		return fmt.Errorf("source work is missing a usable current-hub bootstrap Secret")
	}
	if err := workResourceWritable(work, "", "secrets", spokeAgentNamespace, sourceBootstrapSecretName); err != nil {
		return err
	}

	ci := secretManifestIndex(objects, currentHubBootstrapSecretName)
	if ci != -1 && !sourceSecretHasKubeconfig(objects[ci]) {
		return fmt.Errorf("source work has an unusable current-hub fallback Secret")
	}
	if err := workResourceWritable(work, "", "secrets", spokeAgentNamespace, currentHubBootstrapSecretName); err != nil {
		return err
	}

	registration, found, err := unstructured.NestedMap(klusterlet.Object, "spec", "registrationConfiguration")
	if err != nil {
		return fmt.Errorf("source work has malformed registration configuration")
	}
	if !found {
		registration = map[string]interface{}{}
	}
	driver, _, err := unstructured.NestedMap(registration, "registrationDriver")
	if err != nil || driver["authType"] == "grpc" {
		return fmt.Errorf("source work has unsupported registration driver for MultipleHubs")
	}
	// Validate only the known, non-secret fields before saving them. Reject
	// unknown bootstrap/feature-gate fields rather than journaling arbitrary data.
	bootstrap := operatorv1.BootstrapKubeConfigs{}
	if value, exists := registration["bootstrapKubeConfigs"]; exists {
		if err := decodeNonSecretField(value, &bootstrap); err != nil {
			return fmt.Errorf("source work has unsupported bootstrap configuration")
		}
	}
	gates := []operatorv1.FeatureGate{}
	if value, exists := registration["featureGates"]; exists {
		if err := decodeNonSecretField(value, &gates); err != nil {
			return fmt.Errorf("source work has unsupported feature gates")
		}
	}
	for _, gate := range gates {
		if gate.Feature == "" || (gate.Mode != "" && gate.Mode != "Enable" && gate.Mode != "Disable") {
			return fmt.Errorf("source work has malformed feature gate")
		}
	}
	if bootstrap.Type != "" && bootstrap.Type != operatorv1.LocalSecrets && bootstrap.Type != operatorv1.None {
		return fmt.Errorf("source work has unsupported bootstrap type")
	}

	ti := secretManifestIndex(objects, secret.Name)
	annotations := work.GetAnnotations()
	baseline := sourceWorkBaseline{}
	if encoded := annotations[sourceWorkBaselineAnnotation]; encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &baseline); err != nil || len(encoded) > maxWorkBaselineSize ||
			baseline.MigrationID != event.MigrationId || baseline.Target != secret.Name ||
			baseline.Klusterlet != klusterlet.GetName() || baseline.Restoring {
			return fmt.Errorf("source work belongs to another migration or has an invalid rollback baseline")
		}
	} else {
		if ti != -1 {
			return fmt.Errorf("source work already contains an unowned target bootstrap Secret")
		}
		baseline = sourceWorkBaseline{
			MigrationID: event.MigrationId, Target: secret.Name,
			Klusterlet: klusterlet.GetName(), AddedCurrentHub: ci == -1,
		}
		if value, exists := registration["bootstrapKubeConfigs"]; exists {
			baseline.Bootstrap, err = json.Marshal(value)
			if err != nil {
				return fmt.Errorf("cannot save bootstrap configuration baseline")
			}
		}
		if value, exists := registration["featureGates"]; exists {
			baseline.FeatureGates, err = json.Marshal(value)
			if err != nil {
				return fmt.Errorf("cannot save feature gate baseline")
			}
		}
		encoded, err := json.Marshal(baseline)
		if err != nil || len(encoded) > maxWorkBaselineSize {
			return fmt.Errorf("source work rollback baseline is too large")
		}
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[sourceWorkBaselineAnnotation] = string(encoded)
		work.SetAnnotations(annotations)
	}

	refs := []interface{}{map[string]interface{}{"name": secret.Name}}
	seen := map[string]bool{secret.Name: true}
	if bootstrap.LocalSecrets != nil {
		for _, ref := range bootstrap.LocalSecrets.KubeConfigSecrets {
			if ref.Name == "" {
				return fmt.Errorf("source work has an empty bootstrap reference")
			}
			if seen[ref.Name] {
				continue
			}
			index := secretManifestIndex(objects, ref.Name)
			if index == -1 || !sourceSecretHasKubeconfig(objects[index]) {
				return fmt.Errorf("source work bootstrap reference %s has no usable Secret", ref.Name)
			}
			seen[ref.Name] = true
			refs = append(refs, map[string]interface{}{"name": ref.Name})
		}
	}
	if !seen[currentHubBootstrapSecretName] {
		refs = append(refs, map[string]interface{}{"name": currentHubBootstrapSecretName})
	}
	// Preserve existing local-secret options such as hubConnectionTimeoutSeconds.
	bootstrapConfig, _, err := unstructured.NestedMap(registration, "bootstrapKubeConfigs")
	if err != nil {
		return fmt.Errorf("malformed source bootstrap configuration")
	}
	if bootstrapConfig == nil {
		bootstrapConfig = map[string]interface{}{}
	}
	localSecretsConfig, _, err := unstructured.NestedMap(bootstrapConfig, "localSecretsConfig")
	if err != nil {
		return fmt.Errorf("malformed source local-secret configuration")
	}
	if localSecretsConfig == nil {
		localSecretsConfig = map[string]interface{}{}
	}
	bootstrapConfig["type"] = string(operatorv1.LocalSecrets)
	localSecretsConfig["kubeConfigSecrets"] = refs
	bootstrapConfig["localSecretsConfig"] = localSecretsConfig
	registration["bootstrapKubeConfigs"] = bootstrapConfig
	featureGates := []interface{}{}
	originalGates, _, err := unstructured.NestedSlice(registration, "featureGates")
	if err != nil {
		return fmt.Errorf("malformed source feature gates")
	}
	for i, gate := range gates {
		if gate.Feature != "MultipleHubs" {
			featureGates = append(featureGates, originalGates[i])
		}
	}
	featureGates = append(featureGates, map[string]interface{}{"feature": "MultipleHubs", "mode": "Enable"})
	registration["featureGates"] = featureGates
	if err := unstructured.SetNestedMap(klusterlet.Object, registration, "spec", "registrationConfiguration"); err != nil {
		return fmt.Errorf("cannot set migration registration configuration")
	}
	if err := replaceWorkManifest(work, ki, klusterlet); err != nil {
		return err
	}

	target := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": secret.Name, "namespace": spokeAgentNamespace},
		"type":     string(corev1.SecretTypeOpaque),
		"data":     map[string]interface{}{"kubeconfig": base64.StdEncoding.EncodeToString(secret.Data["kubeconfig"])},
	}}
	if ti != -1 {
		// Preserve target Secret metadata and unrelated keys on retries.
		target = objects[ti]
		secretType, _, err := unstructured.NestedString(target.Object, "type")
		immutable, _, immutableErr := unstructured.NestedBool(target.Object, "immutable")
		if err != nil || immutableErr != nil || immutable || (secretType != "" && secretType != string(corev1.SecretTypeOpaque)) {
			return fmt.Errorf("source work target bootstrap Secret has incompatible type or immutability")
		}
		if err := unstructured.SetNestedField(target.Object, base64.StdEncoding.EncodeToString(secret.Data["kubeconfig"]),
			"data", "kubeconfig"); err != nil {
			return fmt.Errorf("malformed target Secret data")
		}
		unstructured.RemoveNestedField(target.Object, "stringData", "kubeconfig")
		if err := replaceWorkManifest(work, ti, target); err != nil {
			return err
		}
	} else {
		work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{})
		if err := replaceWorkManifest(work, len(work.Spec.Workload.Manifests)-1, target); err != nil {
			return err
		}
	}
	if ci == -1 || baseline.AddedCurrentHub {
		currentHub := objects[si].DeepCopy()
		currentHub.SetName(currentHubBootstrapSecretName)
		if ci == -1 {
			work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{})
			ci = len(work.Spec.Workload.Manifests) - 1
		}
		if err := replaceWorkManifest(work, ci, currentHub); err != nil {
			return err
		}
	}
	return nil
}

func decodeNonSecretField(value interface{}, into interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// DisallowUnknownFields ensures the rollback annotation contains only the
	// typed API's feature names/modes and local Secret references, not credentials.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}

func replaceWorkManifest(work *workv1.ManifestWork, index int, obj *unstructured.Unstructured) error {
	encoded, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("cannot encode source work manifest %d", index)
	}
	// Keep the original bytes when semantically unchanged (including whitespace).
	var original interface{}
	if json.Unmarshal(work.Spec.Workload.Manifests[index].Raw, &original) == nil && reflect.DeepEqual(original, obj.Object) {
		return nil
	}
	work.Spec.Workload.Manifests[index].RawExtension = runtime.RawExtension{Raw: encoded}
	return nil
}

func (s *MigrationSourceSyncer) ensureMigrationSourceWork(ctx context.Context, cluster string,
	event *migration.MigrationSourceBundle, secret *corev1.Secret,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		work := &workv1.ManifestWork{}
		if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
			return sourceWorkAPIError("read", err)
		}
		before := work.DeepCopy()
		if err := prepareSourceWork(work, event, secret); err != nil {
			return err
		}
		if reflect.DeepEqual(before.Spec, work.Spec) && reflect.DeepEqual(before.Annotations, work.Annotations) {
			return nil
		}
		return sourceWorkAPIError("update", apiClient.Update(ctx, work))
	})
}

func (s *MigrationSourceSyncer) checkMigrationSourceWork(ctx context.Context, cluster string,
	event *migration.MigrationSourceBundle, secret *corev1.Secret,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	work := &workv1.ManifestWork{}
	if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
		return sourceWorkAPIError("read", err)
	}
	if work.DeletionTimestamp != nil {
		return fmt.Errorf("source work is being deleted")
	}
	desired := work.DeepCopy()
	if err := prepareSourceWork(desired, event, secret); err != nil {
		return err
	}
	if !reflect.DeepEqual(work.Spec, desired.Spec) || !reflect.DeepEqual(work.Annotations, desired.Annotations) {
		return fmt.Errorf("source work %s/%s generation %d does not contain the target migration payload",
			work.Namespace, work.Name, work.Generation)
	}
	condition := meta.FindStatusCondition(work.Status.Conditions, workv1.WorkApplied)
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.ObservedGeneration < work.Generation || work.Generation < 1 {
		return fmt.Errorf("source work %s/%s generation %d has not reported current-generation Applied=True",
			work.Namespace, work.Name, work.Generation)
	}
	log.Debugf("source migration work %s/%s generation %d Applied=True", work.Namespace, work.Name, work.Generation)
	return nil
}

// restoreMigrationSourceWork is safe for partial initialization and repeat
// rollback. Keep the journal until the restored generation is Applied, so a
// timeout/restart cannot resume the import controller ahead of restoration.
func (s *MigrationSourceSyncer) restoreMigrationSourceWork(ctx context.Context, cluster string,
	event *migration.MigrationSourceBundle,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	restored := false
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		work := &workv1.ManifestWork{}
		if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		encoded := work.Annotations[sourceWorkBaselineAnnotation]
		if encoded == "" {
			return nil
		}
		baseline := sourceWorkBaseline{}
		if len(encoded) > maxWorkBaselineSize || json.Unmarshal([]byte(encoded), &baseline) != nil ||
			baseline.MigrationID != event.MigrationId || baseline.Target != bootstrapSecretNamePrefix+event.ToHub {
			return fmt.Errorf("source work has an invalid or different migration rollback baseline")
		}
		mc := &clusterv1.ManagedCluster{}
		if err := apiClient.Get(ctx, client.ObjectKey{Name: cluster}, mc); err != nil {
			return err
		}
		if !mc.Spec.HubAcceptsClient {
			return fmt.Errorf("cannot restore source work while source admission is disabled")
		}
		before := work.DeepCopy()
		objects, ki, err := decodeSourceWork(work)
		if err != nil {
			return err
		}
		klusterlet := objects[ki]
		if klusterlet.GetName() != baseline.Klusterlet {
			return fmt.Errorf("source work Klusterlet changed during migration")
		}
		for field, raw := range map[string]json.RawMessage{
			"bootstrapKubeConfigs": baseline.Bootstrap, "featureGates": baseline.FeatureGates,
		} {
			if len(raw) == 0 {
				unstructured.RemoveNestedField(klusterlet.Object, "spec", "registrationConfiguration", field)
				continue
			}
			var value interface{}
			if json.Unmarshal(raw, &value) != nil {
				return fmt.Errorf("invalid source work rollback field")
			}
			if err := unstructured.SetNestedField(klusterlet.Object, value, "spec", "registrationConfiguration", field); err != nil {
				return err
			}
		}
		if err := replaceWorkManifest(work, ki, klusterlet); err != nil {
			return err
		}
		// Remove only Secret manifests introduced by this migration. Iterate
		// backwards so deleting one manifest cannot shift another index.
		for i := len(objects) - 1; i >= 0; i-- {
			obj := objects[i]
			if obj.GetAPIVersion() == "v1" && obj.GetKind() == "Secret" && obj.GetNamespace() == spokeAgentNamespace &&
				(obj.GetName() == baseline.Target || (baseline.AddedCurrentHub && obj.GetName() == currentHubBootstrapSecretName)) {
				work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests[:i], work.Spec.Workload.Manifests[i+1:]...)
			}
		}
		baseline.Restoring = true
		journal, err := json.Marshal(baseline)
		if err != nil {
			return fmt.Errorf("cannot save source work restoration journal")
		}
		work.Annotations[sourceWorkBaselineAnnotation] = string(journal)
		restored = true
		if reflect.DeepEqual(before.Spec, work.Spec) && reflect.DeepEqual(before.Annotations, work.Annotations) {
			return nil
		}
		return sourceWorkAPIError("restore", apiClient.Update(ctx, work))
	})
	if err != nil || !restored {
		return err
	}
	if sourceWorkWaitTimeout(ctx) <= 0 {
		return fmt.Errorf("source work restoration deadline expired")
	}
	if err := wait.PollUntilContextTimeout(ctx, time.Second, sourceWorkWaitTimeout(ctx), true,
		func(pollCtx context.Context) (bool, error) {
			work := &workv1.ManifestWork{}
			if err := apiClient.Get(pollCtx, sourceWorkKey(cluster), work); err != nil {
				return false, nil
			}
			return sourceWorkRestorationReady(work, event), nil
		}); err != nil {
		return fmt.Errorf("restored source work for cluster %s has not been applied: %w", cluster, err)
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		work := &workv1.ManifestWork{}
		if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
			return sourceWorkAPIError("read restored work", err)
		}
		if !sourceWorkRestorationReady(work, event) {
			return fmt.Errorf("restored source work changed before cleanup")
		}
		baseline := sourceWorkBaseline{}
		if json.Unmarshal([]byte(work.Annotations[sourceWorkBaselineAnnotation]), &baseline) != nil ||
			baseline.MigrationID != event.MigrationId || !baseline.Restoring {
			return fmt.Errorf("source work restoration journal changed before cleanup")
		}
		delete(work.Annotations, sourceWorkBaselineAnnotation)
		return sourceWorkAPIError("remove restoration journal", apiClient.Update(ctx, work))
	})
}

// API validation errors can include the rejected manifest (and kubeconfig).
// Keep conflict errors for RetryOnConflict, but never log other response bodies.
func sourceWorkAPIError(operation string, err error) error {
	if err == nil || apierrors.IsConflict(err) {
		return err
	}
	return fmt.Errorf("source work %s failed: %s", operation, apierrors.ReasonForError(err))
}

func sourceWorkRestorationReady(work *workv1.ManifestWork, event *migration.MigrationSourceBundle) bool {
	if !sourceWorkApplied(work) {
		return false
	}
	baseline := sourceWorkBaseline{}
	encoded := work.Annotations[sourceWorkBaselineAnnotation]
	if len(encoded) > maxWorkBaselineSize || json.Unmarshal([]byte(encoded), &baseline) != nil ||
		!baseline.Restoring || baseline.MigrationID != event.MigrationId || baseline.Target != bootstrapSecretNamePrefix+event.ToHub {
		return false
	}
	objects, ki, err := decodeSourceWork(work)
	if err != nil || objects[ki].GetName() != baseline.Klusterlet || secretManifestIndex(objects, baseline.Target) != -1 {
		return false
	}
	if baseline.AddedCurrentHub && secretManifestIndex(objects, currentHubBootstrapSecretName) != -1 {
		return false
	}
	si := secretManifestIndex(objects, sourceBootstrapSecretName)
	if si == -1 || !sourceSecretHasKubeconfig(objects[si]) {
		return false
	}
	for field, raw := range map[string]json.RawMessage{"bootstrapKubeConfigs": baseline.Bootstrap, "featureGates": baseline.FeatureGates} {
		value, found, err := unstructured.NestedFieldNoCopy(objects[ki].Object, "spec", "registrationConfiguration", field)
		if err != nil {
			return false
		}
		if len(raw) == 0 {
			if found {
				return false
			}
			continue
		}
		var expected interface{}
		if json.Unmarshal(raw, &expected) != nil || !found || !reflect.DeepEqual(expected, value) {
			return false
		}
	}
	return true
}

func sourceWorkApplied(work *workv1.ManifestWork) bool {
	condition := meta.FindStatusCondition(work.Status.Conditions, workv1.WorkApplied)
	return work.DeletionTimestamp == nil && work.Generation > 0 && condition != nil &&
		condition.Status == metav1.ConditionTrue && condition.ObservedGeneration >= work.Generation
}

func sourceWorkWaitTimeout(ctx context.Context) time.Duration {
	if expiry := expireTimeFromContext(ctx); !expiry.IsZero() {
		return remainingExpireTime(expiry)
	}
	// Production events carry expirytime. Bound direct calls without one too.
	return 30 * time.Second
}
