#!/bin/bash

set -euox pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

start=$(date +%s)

# Init clusters
start_time=$(date +%s)

kind_cluster "$GH_NAME" 2>&1 &
for i in $(seq 1 "${MH_NUM}"); do
  kind_cluster "hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    kind_cluster "hub$i-cluster$j" 2>&1 &
  done
done

wait
echo -e "${YELLOW} creating clusters:${NC} $(($(date +%s) - start_time)) seconds"

# service-ca
enable_service_ca "$GH_NAME" "$TEST_DIR/manifest" 2>&1 || true

# async middlewares
bash "$CURRENT_DIR/e2e_postgres.sh" "$CONFIG_DIR/hub1-cluster1" "$GH_KUBECONFIG" 2>&1 & # install postgres into hub1
echo "$!" >"$CONFIG_DIR/PID"

bash "$CURRENT_DIR/e2e_kafka.sh" "$CONFIG_DIR/hub2-cluster1" "$GH_KUBECONFIG" 2>&1 &
echo "$!" >>"$CONFIG_DIR/PID"

# init hubs
start_time=$(date +%s)

pids=()
init_hub "$GH_NAME" 2>&1 &
pids+=($!)
for i in $(seq 1 "${MH_NUM}"); do
  init_hub "hub$i" 2>&1 &
  pids+=($!)
done
for pid in "${pids[@]}"; do
    wait "$pid" || true
done
echo -e "${YELLOW} initializing hubs:${NC} $(($(date +%s) - start_time)) seconds"

# async ocm, policy and app
start_time=$(date +%s)

# gobal-hub: hub1, hub2
for i in $(seq 1 "${MH_NUM}"); do
  bash "$CURRENT_DIR"/ocm.sh "$GH_NAME" "hub$i" HUB_INIT=false 2>&1 &
  echo "$!" >>"$CONFIG_DIR/PID"
done

# hub1: cluster1 | hub2: cluster1
for i in $(seq 1 "${MH_NUM}"); do
  for j in $(seq 1 "${MC_NUM}"); do
    bash "$CURRENT_DIR"/ocm.sh "hub$i" "hub$i-cluster$j" HUB_INIT=false 2>&1 &
    echo "$!" >>"$CONFIG_DIR/PID"
  done
done

wait
echo -e "${YELLOW} installing ocm, app and policy:${NC} $(($(date +%s) - start_time)) seconds"

# validation
start_time=$(date +%s)

for i in $(seq 1 "${MH_NUM}"); do
  wait_ocm "$GH_NAME" "hub$i"
  wait_policy "$GH_NAME" "hub$i"
  wait_application "$GH_NAME" "hub$i"
  for j in $(seq 1 "${MC_NUM}"); do
    wait_ocm "hub$i" "hub$i-cluster$j"
    wait_policy "hub$i" "hub$i-cluster$j"
    wait_application "hub$i" "hub$i-cluster$j"
  done
done

echo -e "${YELLOW} validating ocm, app and policy:${NC} $(($(date +%s) - start_time)) seconds"

# kubeconfig
for i in $(seq 1 "${MH_NUM}"); do
  echo -e "$CYAN [Access the ManagedHub]: export KUBECONFIG=$CONFIG_DIR/hub$i $NC"
  for j in $(seq 1 "${MC_NUM}"); do
    echo -e "$CYAN [Access the ManagedCluster]: export KUBECONFIG=$CONFIG_DIR/hub$i-cluster$j $NC"
  done
done
echo -e "${BOLD_GREEN}[Access the Clusters]: export KUBECONFIG=$KUBECONFIG $NC"
echo -e "${BOLD_GREEN}[ END ] ${NC} $(($(date +%s) - start)) seconds"