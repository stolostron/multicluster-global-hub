#!/bin/bash
#
# PREREQUISITE:
#  KinD https://kind.sigs.k8s.io/docs/user/quick-start/
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM
# NOTE:
#  the ocm_clean.sh script must be located under the same directory as the ocm_setup.sh
#

HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-1}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-2}

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
PID=${CONFIG_DIR}/pid

touch "$PID"
while read -r line; do
  if [[ $line != "" ]]; then
    kill -9 "$line" >/dev/null 2>&1
  fi
done <"$PID"

rm "$PID" >/dev/null 2>&1

for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  kind delete cluster --name "hub${i}"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    kind delete cluster --name "hub${i}-cluster${j}"
  done
done
rm -rf "$CONFIG_DIR"


