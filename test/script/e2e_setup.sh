#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

# Clean up any stale KUBECONFIG lock files from previous failed runs
if [ -f "${KUBECONFIG}.lock" ]; then
  echo -e "${YELLOW}Removing stale KUBECONFIG lock file: ${KUBECONFIG}.lock${NC}"
  rm -f "${KUBECONFIG}.lock"
fi

start=$(date +%s)

# Init clusters
start_time=$(date +%s)

kind_cluster "$GH_NAME"
for i in $(seq 1 "${MH_NUM}"); do
  kind_cluster "hub$i"
done

echo -e "${YELLOW} creating clusters:${NC} $(($(date +%s) - start_time)) seconds"

# Init hubs
start_time=$(date +%s)
pids=()

# async install olm
enable_olm "$GH_NAME" 2>&1 &
pids+=($!)

init_hub "$GH_NAME" 2>&1 &
pids+=($!)
for i in $(seq 1 "${MH_NUM}"); do
  init_hub "hub$i" 2>&1 &
  pids+=($!)
done
for pid in "${pids[@]}"; do
  wait "$pid" || true
done

# service-ca
# it reports `CSV "packageserver" failed to reach phase succeeded` if create service ca before enable olm
enable_service_ca "$GH_NAME" "$TEST_DIR/manifest" 2>&1 || true

# install the mch on the global hub and managed hubs
install_mch "$GH_NAME"
for i in $(seq 1 "${MH_NUM}"); do
  # install the mch on the managed hub
  install_mch "hub$i"
done
echo -e "${YELLOW} initializing hubs:${NC} $(($(date +%s) - start_time)) seconds"

# Install KlusterletConfig CRD and create multicluster-engine namespace on each hub
# This is required for migration e2e tests in OCM environment
for i in $(seq 1 "${MH_NUM}"); do
  echo -e "${YELLOW}Installing KlusterletConfig CRD on hub$i${NC}"
  kubectl apply -f "$TEST_DIR/manifest/crd/klusterletconfig.yaml" --kubeconfig "$CONFIG_DIR/hub$i" 2>/dev/null || true
  echo -e "${YELLOW}Creating multicluster-engine namespace on hub$i${NC}"
  kubectl create namespace multicluster-engine --kubeconfig "$CONFIG_DIR/hub$i" 2>/dev/null || true
done

# async ocm, policy
start_time=$(date +%s)

# gobal-hub: hub1, hub2
pids=()
for i in $(seq 1 "${MH_NUM}"); do
  bash "$CURRENT_DIR"/ocm.sh "$GH_NAME" "hub$i" HUB_INIT=false POLICY_INIT=false 2>&1 &
  pid=$!
  pids+=($pid)
  echo "$pid" >>"$CONFIG_DIR/PID"
done

# hub1: cluster1 | hub2: cluster1
for i in $(seq 1 "${MH_NUM}"); do
  for j in $(seq 1 "${MC_NUM}"); do
    bash "$CURRENT_DIR"/ocm.sh "hub$i" "hub$i-cluster$j" HUB_INIT=false 2>&1 &
    pid=$!
    pids+=($pid)
    echo "$pid" >>"$CONFIG_DIR/PID"
  done
done

# install the BYO (Bring Your Own) PostgreSQL and Kafka asynchronously during the OCMS installation
bash "$CURRENT_DIR/e2e_postgres.sh" "$CONFIG_DIR/hub1" "$GH_KUBECONFIG" 2>&1 &
pid=$!
pids+=($pid)
echo "$pid" >"$CONFIG_DIR/PID"
bash "$CURRENT_DIR/e2e_kafka.sh" "$CONFIG_DIR/hub2" "$GH_KUBECONFIG" 2>&1 &
pid=$!
pids+=($pid)
echo "$pid" >>"$CONFIG_DIR/PID"

# Wait for all background processes and check for failures
failed=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    echo -e "${RED}Process $pid failed${NC}"
    failed=1
  fi
done

if [ $failed -eq 1 ]; then
  echo -e "${RED}One or more setup processes failed. Exiting...${NC}"
  exit 1
fi

echo -e "${YELLOW} installing ocm and policy:${NC} $(($(date +%s) - start_time)) seconds"

# Install managed-serviceaccount addon on global hub
# This is required for migration functionality to create ServiceAccounts and collect tokens
echo -e "${YELLOW}Installing managed-serviceaccount addon on global hub${NC}"
helm repo add ocm https://open-cluster-management.io/helm-charts 2>/dev/null || true
helm repo update ocm
helm install -n open-cluster-management-addon --create-namespace \
  managed-serviceaccount ocm/managed-serviceaccount --kubeconfig "$GH_KUBECONFIG" 2>/dev/null || true
kubectl wait deployment -n open-cluster-management-addon managed-serviceaccount-addon-manager \
  --for condition=Available=True --timeout=120s --kubeconfig "$GH_KUBECONFIG" || true
echo -e "${YELLOW}managed-serviceaccount addon installed${NC}"

# apply standalone agent
helm install event-exporter "$PROJECT_DIR"/doc/event-exporter -n open-cluster-management --set image="$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF" --set sourceName="event-exporter" --kubeconfig "$GH_KUBECONFIG"

# kubeconfig
for i in $(seq 1 "${MH_NUM}"); do
  echo -e "$CYAN [Access the ManagedHub]: export KUBECONFIG=$CONFIG_DIR/hub$i $NC"
  for j in $(seq 1 "${MC_NUM}"); do
    echo -e "$CYAN [Access the ManagedCluster]: export KUBECONFIG=$CONFIG_DIR/hub$i-cluster$j $NC"
  done
done
echo -e "${BOLD_GREEN}[Access the Clusters]: export KUBECONFIG=$KUBECONFIG $NC"
echo -e "${BOLD_GREEN}[ END ] ${NC} $(($(date +%s) - start)) seconds"
