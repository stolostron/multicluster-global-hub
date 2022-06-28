#!/bin/bash
#
# PREREQUISITE:
#  KinD https://kind.sigs.k8s.io/docs/user/quick-start/
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM

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

for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  kind delete cluster --name "hub${i}"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    kind delete cluster --name "hub${i}-cluster${j}"
  done
done
rm "$PID" >/dev/null 2>&1
ps -ef |grep "setup" |grep -v grep |awk '{print $2}' | xargs kill -9
rm -rf "$CONFIG_DIR"