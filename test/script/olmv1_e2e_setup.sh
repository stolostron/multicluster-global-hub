#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

cluster_name="global-hub-olmv1"
GH_NAMESPACE="multicluster-global-hub"
REGISTRY="localhost:5001"
INITIAL_VERSION="5.0.0"

start=$(date +%s)

# ── 1. Create KinD cluster ──────────────────────────────────────────────────
echo -e "${YELLOW}=== Phase 1: Creating KinD cluster ===${NC}"
kind_cluster "$cluster_name" "kindest/node:v1.32.2"
install_crds "$cluster_name" false
enable_service_ca "$cluster_name" "$TEST_DIR/manifest"

# ── 2. Start local Docker registry on the kind network ──────────────────────
echo -e "${YELLOW}=== Phase 2: Starting local registry ===${NC}"
start_local_registry

# ── 3. Install OLMv0 (for Strimzi coexistence) ─────────────────────────────
echo -e "${YELLOW}=== Phase 3: Installing OLMv0 ===${NC}"
enable_olm "$cluster_name"

# ── 4. Install OLMv1 (cert-manager + operator-controller + catalogd) ───────
echo -e "${YELLOW}=== Phase 4: Installing OLMv1 ===${NC}"
install_olmv1 "$cluster_name"

# ── 5. Build operator, manager, agent images ────────────────────────────────
echo -e "${YELLOW}=== Phase 5: Building component images ===${NC}"
cd "$PROJECT_DIR" || exit
MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF="${REGISTRY}/ghub-operator:v${INITIAL_VERSION}"
MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF="${REGISTRY}/ghub-manager:v${INITIAL_VERSION}"
MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF="${REGISTRY}/ghub-agent:v${INITIAL_VERSION}"

docker build . -t "$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF" -f operator/Dockerfile
docker build . -t "$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF" -f manager/Dockerfile
docker build . -t "$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF" -f agent/Dockerfile

docker push "$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF"
docker push "$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF"
docker push "$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF"

# Also load into kind so the operator can pull manager/agent images
kind load docker-image "$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF" --name "$cluster_name"
kind load docker-image "$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF" --name "$cluster_name"
kind load docker-image "$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF" --name "$cluster_name"

# Update manager.yaml to reference built images with IfNotPresent
sed -i -e "s;quay.io/stolostron/multicluster-global-hub-manager:latest;${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF};" ./operator/config/manager/manager.yaml
sed -i -e "s;quay.io/stolostron/multicluster-global-hub-agent:latest;${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF};" ./operator/config/manager/manager.yaml
sed -i -e "s;imagePullPolicy: Always;imagePullPolicy: IfNotPresent;" ./operator/config/manager/manager.yaml

# ── 6. Build OLM bundle ─────────────────────────────────────────────────────
echo -e "${YELLOW}=== Phase 6: Building OLM bundle ===${NC}"
cd "$PROJECT_DIR/operator" || exit
VERSION="${INITIAL_VERSION}" RELEASE_LINE="5.0" make bundle
# Build and push the bundle image
cat > bundle.Dockerfile <<'BEOF'
FROM scratch
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=multicluster-global-hub-operator
LABEL operators.operatorframework.io.bundle.channels.v1=release-5.0
LABEL operators.operatorframework.io.bundle.channel.default.v1=release-5.0
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
BEOF
docker build -f bundle.Dockerfile -t "${REGISTRY}/ghub-bundle:v${INITIAL_VERSION}" .
docker push "${REGISTRY}/ghub-bundle:v${INITIAL_VERSION}"
rm -f bundle.Dockerfile

# ── 7. Build FBC catalog ────────────────────────────────────────────────────
echo -e "${YELLOW}=== Phase 7: Building File-Based Catalog ===${NC}"
cd "$PROJECT_DIR" || exit
build_fbc_catalog "$INITIAL_VERSION" "" "$REGISTRY"

# ── 8. Create ClusterCatalog ────────────────────────────────────────────────
echo -e "${YELLOW}=== Phase 8: Creating ClusterCatalog ===${NC}"
create_cluster_catalog "$INITIAL_VERSION" "$REGISTRY" "$cluster_name"

# ── 9. Install Global Hub operator via ClusterExtension ─────────────────────
echo -e "${YELLOW}=== Phase 9: Installing operator via ClusterExtension ===${NC}"
install_operator_clusterextension "$GH_NAMESPACE" "$cluster_name"

# ── 10. Create MulticlusterGlobalHub CR ─────────────────────────────────────
echo -e "${YELLOW}=== Phase 10: Deploying MulticlusterGlobalHub ===${NC}"
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${cluster_name}-control-plane")

cat <<EOF | kubectl apply --context "$cluster_name" -f -
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    global-hub.open-cluster-management.io/strimzi-catalog-source-name: operatorhubio-catalog
    global-hub.open-cluster-management.io/strimzi-catalog-source-namespace: olm
    global-hub.open-cluster-management.io/kafka-use-nodeport: ""
    global-hub.open-cluster-management.io/kind-cluster-ip: "${global_hub_node_ip}"
  name: multiclusterglobalhub
  namespace: ${GH_NAMESPACE}
spec:
  availabilityConfig: Basic
  dataLayer:
    kafka:
      topics:
        specTopic: gh-spec
        statusTopic: gh-event.*
      storageSize: 1Gi
    postgres:
      retention: 18m
      storageSize: 1Gi
  enableMetrics: false
  imagePullPolicy: IfNotPresent
EOF

# Trap exit for debug on failure - sanitize logs to avoid exposing secrets
trap 'on_error' EXIT
on_error() {
  echo "Error occurred. Collecting diagnostics..."

  echo "=== ClusterExtension Status ==="
  kubectl get clusterextension -o wide --context "$cluster_name" 2>/dev/null | head -10 || true

  echo "=== ClusterCatalog Status ==="
  kubectl get clustercatalog -o wide --context "$cluster_name" 2>/dev/null | head -10 || true

  echo "=== Pods ==="
  kubectl get pod -n "$GH_NAMESPACE" --context "$cluster_name" 2>/dev/null | head -20 || true

  echo "=== Deployments ==="
  kubectl get deploy -n "$GH_NAMESPACE" --context "$cluster_name" 2>/dev/null || true

  echo "=== Operator Status (redacted logs - check cluster logs for details) ==="
  kubectl get deploy multicluster-global-hub-operator -n "$GH_NAMESPACE" --context "$cluster_name" 2>/dev/null || true
}

# ── 11. Wait for components ─────────────────────────────────────────────────
echo -e "${YELLOW}=== Phase 11: Waiting for components ===${NC}"
wait_cmd "kubectl get deploy/multicluster-global-hub-operator -n $GH_NAMESPACE --context $cluster_name"
wait_cmd "kubectl get deploy/multicluster-global-hub-manager -n $GH_NAMESPACE --context $cluster_name"
kubectl wait deploy/multicluster-global-hub-manager -n "$GH_NAMESPACE" --for condition=Available=True --timeout=180s --context "$cluster_name"

# Restore default behavior
trap - EXIT

echo -e "${GREEN}=== OLMv1 E2E setup complete in $(($(date +%s) - start)) seconds ===${NC}"
echo -e "${GREEN}Operator installed via: ClusterExtension${NC}"
echo -e "${GREEN}Catalog served via:     ClusterCatalog (catalogd)${NC}"
echo -e "${GREEN}OLMv1 installation:     Strimzi Kafka via ClusterExtension${NC}"
