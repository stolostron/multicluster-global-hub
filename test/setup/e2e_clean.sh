#!/bin/bash

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

MH_NUM=${MH_NUM:-2}
MC_NUM=${MC_NUM:-1}

# setup kubeconfig
GH_NAME="global-hub"
KUBE_DIR=${CURRENT_DIR}/kubeconfig
KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/clusters}

while read -r line; do
  if [[ $line != "" ]]; then
    kill -9 "${line}" >/dev/null 2>&1
  fi
done <"$KUBE_DIR/PID"

kind delete cluster --name ${GH_NAME}
for i in $(seq 1 "${MH_NUM}"); do
  kind delete cluster --name "hub${i}"
  for j in $(seq 1 "${MC_NUM}"); do
    kind delete cluster --name "hub${i}-cluster${j}"
  done
done

rm -rf "$KUBE_DIR"
# ps -ef | grep "setup" | grep -v grep |awk '{print $2}' | xargs kill -9 >/dev/null 2>&1

