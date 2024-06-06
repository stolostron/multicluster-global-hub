#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

export CURRENT_DIR
export GH_NAME="global-hub"
export GH_CTX="kind-global-hub"
export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}

# setup kubeconfig
export KUBE_DIR=${CURRENT_DIR}/kubeconfig
check_dir "$KUBE_DIR"
export KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/kind-clusters}

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
# rm -rf "$KUBE_DIR"