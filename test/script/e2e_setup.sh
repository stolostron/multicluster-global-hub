#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
TEST_DIR=$(dirname "$CURRENT_DIR")
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

export CURRENT_DIR
export GH_NAME="global-hub"
export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}
export KinD=true
export CONFIG_DIR=${CURRENT_DIR}/config
export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}
export GH_KUBECONFIG=$CONFIG_DIR/$GH_NAME

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

start=$(date +%s)

# Init clusters
echo -e "$BLUE creating clusters $NC"
start_time=$(date +%s)

kind_cluster "$GH_NAME" 2>&1 
for i in $(seq 1 "${MH_NUM}"); do
  kind_cluster "hub$i" 2>&1
done

echo -e "${YELLOW} creating hubs:${NC} $(($(date +%s) - start_time)) seconds"

# GH
# service-ca
echo -e "$BLUE setting global hub service-ca and middlewares $NC"
enable_service_ca $GH_NAME "$CURRENT_DIR/manifest" 2>&1 || true
# async middlewares
bash "$TEST_DIR/manifest/postgres/postgres_setup.sh" "$GH_KUBECONFIG" 2>&1 &
echo "$!" >"$CONFIG_DIR/PID"
bash "$TEST_DIR"/manifest/kafka/kafka_setup.sh "$GH_KUBECONFIG" 2>&1 &
echo "$!" >>"$CONFIG_DIR/PID"

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

# hub1: cluster1 | hub2: cluster1
for i in $(seq 1 "${MH_NUM}"); do
  (
    init_hub "hub$i" 2>&1
    for j in $(seq 1 "${MC_NUM}"); do
      bash "$CURRENT_DIR"/ocm.sh "hub$i" "hub$i-cluster$j" HUB_INIT=false 2>&1 &
    done
    wait
  ) &
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