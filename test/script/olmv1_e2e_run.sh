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
kubectl wait kafka -n "$GH_NAMESPACE" --all --for=condition=Ready=True --timeout=300s --context "$cluster_name"
echo -e "${GREEN}Kafka: Ready${NC}"

# ── 6. Validate Postgres ────────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking Postgres ---${NC}"
kubectl wait statefulset -n "$GH_NAMESPACE" -l postgres-operator.crunchydata.com/cluster --for=jsonpath='{.status.readyReplicas}'=3 --timeout=300s --context "$cluster_name"
echo -e "${GREEN}Postgres: Ready${NC}"

# ── 7. Validate Grafana ─────────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking Grafana ---${NC}"
kubectl wait deploy/multicluster-global-hub-grafana -n "$GH_NAMESPACE" --for=condition=Available=True --timeout=120s --context "$cluster_name"
echo -e "${GREEN}Grafana: Available${NC}"

# ── 8. Validate MCGH status ─────────────────────────────────────────────────
echo -e "${YELLOW}--- Checking MulticlusterGlobalHub status ---${NC}"
mcgh_phase=$(kubectl get mcgh -n "$GH_NAMESPACE" --context "$cluster_name" -o jsonpath='{.items[0].status.phase}')
if [[ "$mcgh_phase" != "Running" ]]; then
  echo "ERROR: MulticlusterGlobalHub phase is '${mcgh_phase}', expected 'Running'"
  exit 1
fi
echo -e "${GREEN}MulticlusterGlobalHub: ${mcgh_phase}${NC}"

# ── 9. Validate Strimzi ClusterExtension (OLMv1 coexistence) ────────────────
echo -e "${YELLOW}--- Checking Strimzi ClusterExtension (OLMv1) ---${NC}"
kubectl wait clusterextension/strimzi-kafka-operator --for=condition=Installed=True --timeout=60s --context "$cluster_name"
echo -e "${GREEN}Strimzi: Installed via OLMv1 ClusterExtension${NC}"

echo "All ClusterExtensions:"
kubectl get clusterextension --context "$cluster_name"

echo -e "${GREEN}=== OLMv1 E2E Validation Complete ===${NC}"
