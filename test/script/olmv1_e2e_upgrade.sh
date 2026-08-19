#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

cluster_name="global-hub-olmv1"
GH_NAMESPACE="multicluster-global-hub"
REGISTRY="localhost:5001"
OLD_VERSION="5.0.0"
NEW_VERSION="5.0.1"

echo -e "${YELLOW}=== OLMv1 Upgrade Test: v${OLD_VERSION} → v${NEW_VERSION} ===${NC}"

# ── 1. Verify current version ──────────────────────────────────────────────
echo -e "${YELLOW}Verifying current operator version...${NC}"
current_version=$(verify_operator_version "$cluster_name")
echo -e "Current version: ${current_version}"

# ── 2. Build bundle v5.0.1 ─────────────────────────────────────────────────
echo -e "${YELLOW}Building bundle v${NEW_VERSION}...${NC}"
cd "$PROJECT_DIR/operator" || exit

# Save original bundle and build the new version
cp -r bundle bundle-backup

VERSION="${NEW_VERSION}" RELEASE_LINE="5.0" make bundle || true

# Update CSV replaces field for upgrade edge
sed -i "/^  name: multicluster-global-hub-operator.v${NEW_VERSION}/a\\  replaces: multicluster-global-hub-operator.v${OLD_VERSION}" \
  bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml 2>/dev/null || true

# Build and push the new bundle image
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
docker build -f bundle.Dockerfile -t "${REGISTRY}/ghub-bundle:v${NEW_VERSION}" .
docker push "${REGISTRY}/ghub-bundle:v${NEW_VERSION}"
rm -f bundle.Dockerfile

# Restore original bundle
rm -rf bundle
mv bundle-backup bundle

# ── 3. Build upgrade catalog (contains both versions) ──────────────────────
echo -e "${YELLOW}Building upgrade catalog...${NC}"
cd "$PROJECT_DIR" || exit
build_fbc_catalog_upgrade "$NEW_VERSION" "$OLD_VERSION" "$REGISTRY"

# ── 4. Update ClusterCatalog to trigger upgrade ────────────────────────────
echo -e "${YELLOW}Updating ClusterCatalog to trigger upgrade...${NC}"
update_cluster_catalog "$NEW_VERSION" "$REGISTRY" "$cluster_name"

# ── 5. Wait for upgrade to complete ────────────────────────────────────────
echo -e "${YELLOW}Waiting for operator upgrade...${NC}"
# operator-controller detects the new catalog content and upgrades
kubectl wait clusterextension/multicluster-global-hub-operator \
  --for=condition=Installed=True --timeout=300s --context "$cluster_name"

# ── 6. Verify new version ──────────────────────────────────────────────────
echo -e "${YELLOW}Verifying upgraded version...${NC}"
new_version=$(verify_operator_version "$cluster_name")
echo -e "Upgraded version: ${new_version}"

# ── 7. Verify components are still healthy ──────────────────────────────────
echo -e "${YELLOW}Verifying components are healthy post-upgrade...${NC}"
wait_cmd "kubectl get deploy/multicluster-global-hub-operator -n $GH_NAMESPACE --context $cluster_name"
kubectl wait deploy/multicluster-global-hub-operator -n "$GH_NAMESPACE" \
  --for=condition=Available=True --timeout=120s --context "$cluster_name"

wait_cmd "kubectl get deploy/multicluster-global-hub-manager -n $GH_NAMESPACE --context $cluster_name"
kubectl wait deploy/multicluster-global-hub-manager -n "$GH_NAMESPACE" \
  --for=condition=Available=True --timeout=120s --context "$cluster_name"

echo -e "${GREEN}=== Upgrade test passed: v${OLD_VERSION} → v${NEW_VERSION} ===${NC}"
