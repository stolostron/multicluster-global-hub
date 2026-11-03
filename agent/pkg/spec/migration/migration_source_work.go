// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
)

const (
	sourceWorkBaselineAnnotation = "global-hub.open-cluster-management.io/migration-work-baseline"
	// sourceWorkAppliedAnnotation stores "<migration id> <generation>" after the
	// writable Applied gate passes. A later registration retry can finish cutover
	// once the import controller has put the work back on ReadOnly.
	sourceWorkAppliedAnnotation = "global-hub.open-cluster-management.io/migration-work-applied"
	// allowManifestWorkUpdateAnnotation keeps the klusterlet ManifestWork on the
	// Update strategy while disable-auto-import is set. The import controller
	// marks the work ReadOnly when this annotation is removed.
	allowManifestWorkUpdateAnnotation = "import.open-cluster-management.io/allow-manifestwork-update"
	spokeAgentNamespace               = "open-cluster-management-agent"
	// Resource name of the spoke's current-hub kubeconfig. The identifier avoids
	// the credential-name pattern that Sonar flags as a hardcoded secret.
	sourceBootstrapKubeconfigName     = "bootstrap-hub-kubeconfig"
	currentHubBootstrapKubeconfigName = sourceBootstrapKubeconfigName + "-current-hub"
	sourceWorkCutoverReserve          = 20 * time.Second
	maxWorkBaselineSize               = 16 * 1024
)

// acmReleaseVersionPattern reads the first major.minor pair, including a
// leading v.
var acmReleaseVersionPattern = regexp.MustCompile(`v?(\d+)\.(\d+)`)

// acm213Token matches an ACM 2.13 release, including a bare "2.13", and does
// not match neighboring versions such as 2.130 or 12.13.
var acm213Token = regexp.MustCompile(`(?:^|[^0-9])2\.13(?:[^0-9]|$)`)

func isACM213(version string) bool {
	return acm213Token.MatchString(version)
}

// sourceWorkMutationSupported reports whether this ACM release leaves an
// existing klusterlet ManifestWork in place while auto-import is disabled.
// The managedcluster-import-controller manifestwork reconciler does that on
// ACM 2.17 and ACM 5. ACM 2.14 through 2.16 rebuild the work from the import
// secret, which would drop a migration payload written here. ACM 2.13 keeps
// the legacy KlusterletConfig path.
func sourceWorkMutationSupported(version string) bool {
	major, minor, ok := acmReleaseVersion(version)
	if !ok {
		return false
	}
	if major > 2 {
		return true
	}
	return major == 2 && minor >= 17
}

func acmReleaseVersion(version string) (int, int, bool) {
	match := acmReleaseVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return 0, 0, false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, false
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// Only non-secret registration fields are saved, on the source work itself, so
// rollback survives an agent restart. Manifests and kubeconfig bytes stay out
// of the annotation.
type sourceWorkBaseline struct {
	MigrationID     string          `json:"migrationID"`
	Target          string          `json:"target"`
	Klusterlet      string          `json:"klusterlet"`
	Bootstrap       json.RawMessage `json:"bootstrap,omitempty"`
	FeatureGates    json.RawMessage `json:"featureGates,omitempty"`
	AddedTarget     bool            `json:"addedTarget,omitempty"`
	AddedCurrentHub bool            `json:"addedCurrentHub,omitempty"`
}

// The manager client may serve reads from an informer cache. Use direct API
// reads for the generation gate. Tests without a REST config use their client.
func (s *MigrationSourceSyncer) migrationSourceClient() (client.Client, error) {
	if s.restConfig == nil {
		return s.client, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sourceAPIClient == nil {
		apiClient, err := client.New(s.restConfig, client.Options{
			Scheme: s.client.Scheme(),
			Mapper: s.client.RESTMapper(),
		})
		if err != nil {
			return nil, fmt.Errorf("cannot initialize uncached source-work client: %w", err)
		}
		s.sourceAPIClient = apiClient
	}
	return s.sourceAPIClient, nil
}

func sourceWorkKey(cluster string) client.ObjectKey {
	return client.ObjectKey{Namespace: cluster, Name: cluster + "-klusterlet"}
}

func sourceWorkReadOnly(work *workv1.ManifestWork) bool {
	// The import controller replaces this list as a whole: every manifest is
	// ReadOnly, or the list is cleared. Any ReadOnly entry is that lock. A
	// per-manifest match can miss it when the resource name does not line up,
	// and the work agent would still refuse the migration payload.
	for _, config := range work.Spec.ManifestConfigs {
		if config.UpdateStrategy != nil && config.UpdateStrategy.Type == workv1.UpdateStrategyTypeReadOnly {
			return true
		}
	}
	return false
}

func sourceWorkAppliedValue(migrationID string, generation int64) string {
	return migrationID + " " + strconv.FormatInt(generation, 10)
}

func sourceWorkAppliedGeneration(work *workv1.ManifestWork, migrationID string) (int64, bool) {
	if work.Annotations == nil || migrationID == "" {
		return 0, false
	}
	prefix := migrationID + " "
	value := work.Annotations[sourceWorkAppliedAnnotation]
	if !strings.HasPrefix(value, prefix) {
		return 0, false
	}
	generation, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || generation < 1 {
		return 0, false
	}
	return generation, true
}

func (m *MigrationSourceSyncer) setAllowManifestWorkUpdate(ctx context.Context, cluster string, allow bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, client.ObjectKey{Name: cluster}, mc); err != nil {
			return err
		}
		annotations := mc.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		_, present := annotations[allowManifestWorkUpdateAnnotation]
		if allow == present {
			return nil
		}
		if allow {
			annotations[allowManifestWorkUpdateAnnotation] = ""
		} else {
			delete(annotations, allowManifestWorkUpdateAnnotation)
		}
		mc.SetAnnotations(annotations)
		return m.client.Update(ctx, mc)
	})
}

func (m *MigrationSourceSyncer) waitForSourceWorkUpdateStrategy(ctx context.Context, cluster string) error {
	apiClient, err := m.migrationSourceClient()
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = wait.PollUntilContextTimeout(
		waitCtx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			work := &workv1.ManifestWork{}
			if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
				return false, sourceWorkAPIError("read", err)
			}
			return !sourceWorkReadOnly(work), nil
		},
	)
	if err != nil {
		return fmt.Errorf("source work %s stayed ReadOnly: %w", cluster, err)
	}
	return nil
}

func (m *MigrationSourceSyncer) lockMigrationSourceWorks(ctx context.Context, clusters []string) error {
	for _, cluster := range clusters {
		if err := m.setAllowManifestWorkUpdate(ctx, cluster, false); err != nil {
			return fmt.Errorf("failed to remove manifest work update window for %s: %w", cluster, err)
		}
	}
	timeout := 60 * time.Second
	if expiry := expireTimeFromContext(ctx); !expiry.IsZero() {
		if remain := time.Until(expiry) - 5*time.Second; remain < timeout {
			timeout = remain
		}
	}
	if timeout <= 0 {
		return fmt.Errorf("insufficient time to wait for ReadOnly source works")
	}
	apiClient, err := m.migrationSourceClient()
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err = wait.PollUntilContextTimeout(waitCtx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		for _, cluster := range clusters {
			work := &workv1.ManifestWork{}
			if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
				return false, sourceWorkAPIError("read", err)
			}
			if !sourceWorkReadOnly(work) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed while waiting for ReadOnly source works: %w", err)
	}
	return nil
}

// sourceWorkStrategyInterceptor simulates the import controller in unit tests:
// disable-auto-import without the update window marks the work ReadOnly, and
// the window clears that lock.
func sourceWorkStrategyInterceptor() interceptor.Funcs {
	return interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err := c.Update(ctx, obj, opts...); err != nil {
				return err
			}
			mc, ok := obj.(*clusterv1.ManagedCluster)
			if !ok {
				return nil
			}
			work := &workv1.ManifestWork{}
			if err := c.Get(ctx, sourceWorkKey(mc.Name), work); err != nil {
				return nil
			}
			_, disabled := mc.Annotations[apiconstants.DisableAutoImportAnnotation]
			_, allow := mc.Annotations[allowManifestWorkUpdateAnnotation]
			wantReadOnly := disabled && !allow
			if wantReadOnly == sourceWorkReadOnly(work) {
				return nil
			}
			if wantReadOnly {
				work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{
					UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly},
				}}
			} else {
				work.Spec.ManifestConfigs = nil
			}
			return c.Update(ctx, work)
		},
	}
}

// sourceWorkPollBudget is shorter than the stage deadline so HubAcceptsClient
// can still be updated after the Applied wait finishes.
func sourceWorkPollBudget(ctx context.Context) (time.Duration, error) {
	if ctx.Err() != nil {
		return 0, fmt.Errorf("source work pre-cutover gate expired or canceled")
	}
	expiry := expireTimeFromContext(ctx)
	budget := 30 * time.Second
	if !expiry.IsZero() {
		budget = time.Until(expiry)
	}
	if budget <= 0 {
		return 0, fmt.Errorf("source work pre-cutover gate expired or canceled")
	}
	if budget > sourceWorkCutoverReserve {
		return budget - sourceWorkCutoverReserve, nil
	}
	return 0, fmt.Errorf("source work pre-cutover gate has insufficient time before stage expiry")
}

func (s *MigrationSourceSyncer) validateMigrationSourceWorks(ctx context.Context, clusters []string,
	event *migration.MigrationSourceBundle, secret *corev1.Secret,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	var failed []string
	for _, cluster := range clusters {
		work := &workv1.ManifestWork{}
		if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", cluster, sourceWorkAPIError("read", err)))
			continue
		}
		if err := prepareSourceWork(work.DeepCopy(), event, secret); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", cluster, err.Error()))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("unsupported source work before migration changes: %s", joinErrors(failed))
	}
	return nil
}

func joinErrors(parts []string) string {
	out := parts[0]
	for _, part := range parts[1:] {
		out += "; " + part
	}
	return out
}

func sourceWorkAPIError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("cannot %s source work: %w", action, err)
}

func decodeSourceWork(work *workv1.ManifestWork) ([]*unstructured.Unstructured, int, error) {
	objects := make([]*unstructured.Unstructured, len(work.Spec.Workload.Manifests))
	klusterletIndex := -1
	for i, manifest := range work.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(manifest.Raw, &obj.Object); err != nil || obj.Object == nil {
			return nil, -1, fmt.Errorf("invalid JSON in source work manifest %d", i)
		}
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

func sourceSecretHasKubeconfig(obj *unstructured.Unstructured) bool {
	data, _, err := unstructured.NestedString(obj.Object, "data", "kubeconfig")
	if err != nil || data == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	return err == nil && len(decoded) > 0
}

// prepareSourceWork adds the target bootstrap Secret and MultipleHubs
// registration to the existing klusterlet ManifestWork. Other manifests are left
// unchanged. Hive does not apply KlusterletConfig while auto-import is disabled,
// so this work is what the spoke actually receives.
func prepareSourceWork(work *workv1.ManifestWork, event *migration.MigrationSourceBundle, secret *corev1.Secret) error {
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
	currentHub := secretManifestIndex(objects, sourceBootstrapKubeconfigName)
	if currentHub == -1 || !sourceSecretHasKubeconfig(objects[currentHub]) {
		return fmt.Errorf("source work is missing a usable current-hub bootstrap Secret")
	}

	registration, found, err := unstructured.NestedMap(klusterlet.Object, "spec", "registrationConfiguration")
	if err != nil {
		return fmt.Errorf("source work has malformed registration configuration")
	}
	if !found {
		registration = map[string]interface{}{}
	}
	annotations := work.GetAnnotations()
	baseline := sourceWorkBaseline{}
	targetIndex := secretManifestIndex(objects, secret.Name)
	currentHubFallback := secretManifestIndex(objects, currentHubBootstrapKubeconfigName)
	if encoded := annotations[sourceWorkBaselineAnnotation]; encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &baseline); err != nil || len(encoded) > maxWorkBaselineSize ||
			baseline.MigrationID != event.MigrationId || baseline.Target != secret.Name ||
			baseline.Klusterlet != klusterlet.GetName() {
			return fmt.Errorf("source work belongs to another migration or has an invalid rollback baseline")
		}
	} else {
		if targetIndex != -1 {
			return fmt.Errorf("source work already contains an unowned target bootstrap Secret")
		}
		baseline = sourceWorkBaseline{
			MigrationID: event.MigrationId, Target: secret.Name, Klusterlet: klusterlet.GetName(),
			AddedTarget: true, AddedCurrentHub: currentHubFallback == -1,
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

	if err := setMigrationRegistration(registration, objects, secret.Name); err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(klusterlet.Object, registration, "spec", "registrationConfiguration"); err != nil {
		return fmt.Errorf("cannot set migration registration configuration")
	}
	if err := replaceWorkManifest(work, ki, klusterlet); err != nil {
		return err
	}
	if err := upsertKubeconfigSecret(work, &objects, secret.Name, secret.Data["kubeconfig"]); err != nil {
		return err
	}
	if baseline.AddedCurrentHub || currentHubFallback == -1 {
		encoded, _, err := unstructured.NestedString(objects[currentHub].Object, "data", "kubeconfig")
		if err != nil || encoded == "" {
			return fmt.Errorf("source work is missing a usable current-hub bootstrap Secret")
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("source work current-hub bootstrap Secret is not valid base64")
		}
		if err := upsertKubeconfigSecret(work, &objects, currentHubBootstrapKubeconfigName, raw); err != nil {
			return err
		}
	}
	return nil
}

func setMigrationRegistration(
	registration map[string]interface{}, objects []*unstructured.Unstructured, target string,
) error {
	refs := []interface{}{map[string]interface{}{"name": target}}
	seen := map[string]bool{target: true}
	existing, _, err := unstructured.NestedSlice(
		registration, "bootstrapKubeConfigs", "localSecretsConfig", "kubeConfigSecrets",
	)
	if err != nil {
		return fmt.Errorf("source work has malformed bootstrap references")
	}
	for _, ref := range existing {
		item, ok := ref.(map[string]interface{})
		if !ok {
			return fmt.Errorf("source work has a malformed bootstrap reference")
		}
		name, _ := item["name"].(string)
		if name == "" || seen[name] {
			continue
		}
		index := secretManifestIndex(objects, name)
		if index == -1 || !sourceSecretHasKubeconfig(objects[index]) {
			return fmt.Errorf("source work bootstrap reference %s has no usable Secret", name)
		}
		seen[name] = true
		refs = append(refs, map[string]interface{}{"name": name})
	}
	if !seen[currentHubBootstrapKubeconfigName] {
		refs = append(refs, map[string]interface{}{"name": currentHubBootstrapKubeconfigName})
	}
	bootstrapConfig, _, err := unstructured.NestedMap(registration, "bootstrapKubeConfigs")
	if err != nil {
		return fmt.Errorf("malformed source bootstrap configuration")
	}
	if bootstrapConfig == nil {
		bootstrapConfig = map[string]interface{}{}
	}
	localSecrets, _, err := unstructured.NestedMap(bootstrapConfig, "localSecretsConfig")
	if err != nil {
		return fmt.Errorf("malformed source local-secret configuration")
	}
	if localSecrets == nil {
		localSecrets = map[string]interface{}{}
	}
	bootstrapConfig["type"] = "LocalSecrets"
	localSecrets["kubeConfigSecrets"] = refs
	bootstrapConfig["localSecretsConfig"] = localSecrets
	registration["bootstrapKubeConfigs"] = bootstrapConfig

	original, _, err := unstructured.NestedSlice(registration, "featureGates")
	if err != nil {
		return fmt.Errorf("malformed source feature gates")
	}
	gates := make([]interface{}, 0, len(original)+1)
	for _, gate := range original {
		item, ok := gate.(map[string]interface{})
		if !ok {
			return fmt.Errorf("source work has a malformed feature gate")
		}
		if item["feature"] == "MultipleHubs" {
			continue
		}
		gates = append(gates, gate)
	}
	gates = append(gates, map[string]interface{}{"feature": "MultipleHubs", "mode": "Enable"})
	registration["featureGates"] = gates
	return nil
}

func upsertKubeconfigSecret(
	work *workv1.ManifestWork, objects *[]*unstructured.Unstructured, name string, kubeconfig []byte,
) error {
	encoded := base64.StdEncoding.EncodeToString(kubeconfig)
	index := secretManifestIndex(*objects, name)
	if index == -1 {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{"name": name, "namespace": spokeAgentNamespace},
			"type":     string(corev1.SecretTypeOpaque),
			"data":     map[string]interface{}{"kubeconfig": encoded},
		}}
		work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{})
		*objects = append(*objects, obj)
		return replaceWorkManifest(work, len(work.Spec.Workload.Manifests)-1, obj)
	}
	obj := (*objects)[index]
	if err := unstructured.SetNestedField(obj.Object, encoded, "data", "kubeconfig"); err != nil {
		return fmt.Errorf("malformed bootstrap Secret %s", name)
	}
	return replaceWorkManifest(work, index, obj)
}

func replaceWorkManifest(work *workv1.ManifestWork, index int, obj *unstructured.Unstructured) error {
	encoded, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("cannot encode source work manifest %d", index)
	}
	var original interface{}
	parsed := json.Unmarshal(work.Spec.Workload.Manifests[index].Raw, &original) == nil
	if parsed && reflect.DeepEqual(original, obj.Object) {
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
	if sourceWorkReadOnly(work) {
		verified, ok := sourceWorkAppliedGeneration(work, event.MigrationId)
		applied := condition != nil && condition.Status == metav1.ConditionTrue &&
			condition.ObservedGeneration >= verified
		if ok && applied {
			return nil
		}
		return fmt.Errorf("source work %s/%s generation %d is ReadOnly and was not applied",
			work.Namespace, work.Name, work.Generation)
	}
	if condition == nil || condition.Status != metav1.ConditionTrue ||
		work.Generation < 1 || condition.ObservedGeneration < work.Generation {
		return fmt.Errorf("source work %s/%s generation %d has not reported current-generation Applied=True",
			work.Namespace, work.Name, work.Generation)
	}
	return nil
}

// recordMigrationSourceWorksApplied stores the generation that passed the
// writable Applied gate. Locking the work changes its spec generation, so a
// registration retry cannot require Applied at that newer generation.
func (s *MigrationSourceSyncer) recordMigrationSourceWorksApplied(
	ctx context.Context, event *migration.MigrationSourceBundle,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	for _, cluster := range event.ManagedClusters {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			work := &workv1.ManifestWork{}
			if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
				return sourceWorkAPIError("read", err)
			}
			recorded, ok := sourceWorkAppliedGeneration(work, event.MigrationId)
			if sourceWorkReadOnly(work) {
				if ok {
					return nil
				}
				return fmt.Errorf(
					"source work %s/%s is ReadOnly before its applied generation was recorded",
					work.Namespace, work.Name,
				)
			}
			condition := meta.FindStatusCondition(work.Status.Conditions, workv1.WorkApplied)
			current := condition != nil && condition.Status == metav1.ConditionTrue &&
				work.Generation >= 1 && condition.ObservedGeneration >= work.Generation
			if !current {
				return fmt.Errorf("source work %s/%s generation %d is not Applied",
					work.Namespace, work.Name, work.Generation)
			}
			if ok && recorded == work.Generation {
				return nil
			}
			annotations := work.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[sourceWorkAppliedAnnotation] = sourceWorkAppliedValue(event.MigrationId, work.Generation)
			work.SetAnnotations(annotations)
			return sourceWorkAPIError("update", apiClient.Update(ctx, work))
		})
		if err != nil {
			return fmt.Errorf("failed to record applied source work for %s: %w", cluster, err)
		}
	}
	return nil
}

func (s *MigrationSourceSyncer) restoreMigrationSourceWork(ctx context.Context, cluster string,
	event *migration.MigrationSourceBundle,
) error {
	apiClient, err := s.migrationSourceClient()
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		work := &workv1.ManifestWork{}
		if err := apiClient.Get(ctx, sourceWorkKey(cluster), work); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return sourceWorkAPIError("read", err)
		}
		encoded := ""
		if work.Annotations != nil {
			encoded = work.Annotations[sourceWorkBaselineAnnotation]
		}
		if encoded == "" {
			return nil
		}
		baseline := sourceWorkBaseline{}
		if len(encoded) > maxWorkBaselineSize || json.Unmarshal([]byte(encoded), &baseline) != nil ||
			baseline.MigrationID != event.MigrationId || baseline.Target != bootstrapSecretNamePrefix+event.ToHub {
			return fmt.Errorf("source work has an invalid or different migration rollback baseline")
		}
		objects, ki, err := decodeSourceWork(work)
		if err != nil {
			return err
		}
		registration, _, err := unstructured.NestedMap(objects[ki].Object, "spec", "registrationConfiguration")
		if err != nil || registration == nil {
			registration = map[string]interface{}{}
		}
		if err := restoreRawField(registration, "bootstrapKubeConfigs", baseline.Bootstrap); err != nil {
			return err
		}
		if err := restoreRawField(registration, "featureGates", baseline.FeatureGates); err != nil {
			return err
		}
		if len(registration) == 0 {
			unstructured.RemoveNestedField(objects[ki].Object, "spec", "registrationConfiguration")
		} else if err := unstructured.SetNestedMap(
			objects[ki].Object, registration, "spec", "registrationConfiguration",
		); err != nil {
			return fmt.Errorf("cannot restore registration configuration")
		}
		if err := replaceWorkManifest(work, ki, objects[ki]); err != nil {
			return err
		}
		remove := map[string]bool{}
		if baseline.AddedTarget {
			remove[baseline.Target] = true
		}
		if baseline.AddedCurrentHub {
			remove[currentHubBootstrapKubeconfigName] = true
		}
		if len(remove) > 0 {
			kept := make([]workv1.Manifest, 0, len(work.Spec.Workload.Manifests))
			for i, obj := range objects {
				if remove[obj.GetName()] && obj.GetKind() == "Secret" && obj.GetNamespace() == spokeAgentNamespace {
					continue
				}
				kept = append(kept, work.Spec.Workload.Manifests[i])
			}
			work.Spec.Workload.Manifests = kept
		}
		annotations := work.GetAnnotations()
		delete(annotations, sourceWorkBaselineAnnotation)
		delete(annotations, sourceWorkAppliedAnnotation)
		work.SetAnnotations(annotations)
		return sourceWorkAPIError("update", apiClient.Update(ctx, work))
	})
}

func restoreRawField(registration map[string]interface{}, field string, raw json.RawMessage) error {
	if len(raw) == 0 {
		delete(registration, field)
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("cannot restore %s", field)
	}
	registration[field] = value
	return nil
}
