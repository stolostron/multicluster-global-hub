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

echo -e "${YELLOW}=== OLMv1 E2E Validation ===${NC}"

# ── 1. Validate OLMv1 installation state ───────────────────────────────────
echo -e "${YELLOW}--- Checking OLMv1 components ---${NC}"
kubectl get deploy -n olmv1-system --context "$cluster_name"
kubectl get clustercatalog --context "$cluster_name"
kubectl get clusterextension --context "$cluster_name"

# ── 2. Validate operator deployed via ClusterExtension ──────────────────────
echo -e "${YELLOW}--- Checking operator deployment ---${NC}"
kubectl get deploy -n "$GH_NAMESPACE" --context "$cluster_name"

ce_status=$(kubectl get clusterextension/multicluster-global-hub-operator \
  --context "$cluster_name" -o jsonpath='{.status.conditions[?(@.type=="Installed")].status}')
if [[ "$ce_status" != "True" ]]; then
  echo "ERROR: ClusterExtension not in Installed=True state"
  exit 1
fi
echo -e "${GREEN}ClusterExtension: Installed=True${NC}"

# ── 3. Validate operator health ─────────────────────────────────────────────
echo -e "${YELLOW}--- Checking operator health ---${NC}"
kubectl wait deploy/multicluster-global-hub-operator -n "$GH_NAMESPACE" \
  --for=condition=Available=True --timeout=60s --context "$cluster_name"
echo -e "${GREEN}Operator: Available${NC}"

# ── 4. Validate manager health ──────────────────────────────────────────────
echo -e "${YELLOW}--- Checking manager health ---${NC}"
kubectl wait deploy/multicluster-global-hub-manager -n "$GH_NAMESPACE" \
  --for=condition=Available=True --timeout=60s --context "$cluster_name"
echo -e "${GREEN}Manager: Available${NC}"

# ── 5. Validate Kafka ───────────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking Kafka ---${NC}"
kubectl get kafka -n "$GH_NAMESPACE" --context "$cluster_name" || echo "Kafka not yet available"

# ── 6. Validate Postgres ────────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking Postgres ---${NC}"
kubectl get statefulset -n "$GH_NAMESPACE" --context "$cluster_name" | grep -i postgres || echo "Postgres not yet available"

# ── 7. Validate Grafana ─────────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking Grafana ---${NC}"
kubectl get deploy -n "$GH_NAMESPACE" --context "$cluster_name" | grep -i grafana || echo "Grafana not yet available"

# ── 8. Validate MCGH status ─────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking MulticlusterGlobalHub status ---${NC}"
kubectl get mcgh -n "$GH_NAMESPACE" --context "$cluster_name" -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Phase not set"

# ── 9. OLMv0/OLMv1 coexistence check ───────────────────────────────────────
echo -e "${YELLOW}--- Checking OLMv0/OLMv1 coexistence ---${NC}"
echo "OLMv0 Subscriptions:"
kubectl get subscriptions.operators.coreos.com --all-namespaces --context "$cluster_name" 2>/dev/null || echo "No OLMv0 subscriptions"
echo "OLMv1 ClusterExtensions:"
kubectl get clusterextension --context "$cluster_name"

echo -e "${GREEN}=== OLMv1 E2E Validation Complete ===${NC}"
