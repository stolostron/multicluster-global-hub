#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

start=$(date +%s)

# Init clusters
echo -e "$BLUE creating clusters $NC"
start_time=$(date +%s)

# GH
kind_cluster "$GH_NAME" 2>&1

# service-ca
echo -e "$BLUE setting global hub service-ca and middlewares $NC"
enable_service_ca "$GH_NAME" "$TEST_DIR/manifest" 2>&1 || true
bash "$CURRENT_DIR/e2e_postgres.sh" "$GH_KUBECONFIG" 2>&1 & # async middlewares
echo "$!" >"$CONFIG_DIR/PID"
bash "$CURRENT_DIR/e2e_kafka.sh" "$GH_KUBECONFIG" 2>&1 &
echo "$!" >>"$CONFIG_DIR/PID"

for i in $(seq 1 "${MH_NUM}"); do
  kind_cluster "hub$i" 2>&1
done

echo -e "${YELLOW} creating hubs:${NC} $(($(date +%s) - start_time)) seconds"

# async ocm, policy and app
echo -e "$BLUE installing ocm, policy, and app in global hub and managed hubs $NC"
start_time=$(date +%s)

# gobal-hub: hub1, hub2
(
  init_hub $GH_NAME 2>&1
  for i in $(seq 1 "${MH_NUM}"); do
    bash "$CURRENT_DIR"/ocm.sh "$GH_NAME" "hub$i" HUB_INIT=false 2>&1 &
  done
  wait
) &
echo "$!" >>"$CONFIG_DIR/PID"

# hub1: cluster1 | hub2: cluster1
for i in $(seq 1 "${MH_NUM}"); do
  (
    init_hub "hub$i" 2>&1
    for j in $(seq 1 "${MC_NUM}"); do
      bash "$CURRENT_DIR"/ocm.sh "hub$i" "hub$i-cluster$j" HUB_INIT=false 2>&1 &
    done
    wait
  ) &
  echo "$!" >>"$CONFIG_DIR/PID"
done

wait
echo -e "${YELLOW} installing ocm, app and policy:${NC} $(($(date +%s) - start_time)) seconds"

# validation
echo -e "$BLUE validating ocm, app and policy $NC"
start_time=$(date +%s)

for i in $(seq 1 "${MH_NUM}"); do
  wait_ocm $GH_NAME "hub$i"
  wait_policy $GH_NAME "hub$i"
  wait_application $GH_NAME "hub$i"
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