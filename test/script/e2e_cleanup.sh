#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

# whether delete the kind clusters
DELETE=${DELETE:-true} 

# setup kubeconfig
KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}

while read -r line; do
  if [[ $line != "" ]]; then
    kill -9 "${line}" >/dev/null 2>&1
  fi
done <"$CONFIG_DIR/PID"

[ "$DELETE" = false ] && exit 0

kind delete cluster --name ${GH_NAME}
for i in $(seq 1 "${MH_NUM}"); do
  kind delete cluster --name "hub${i}"
  rm "$CONFIG_DIR/hub${i}"
  for j in $(seq 1 "${MC_NUM}"); do
    kind delete cluster --name "hub${i}-cluster${j}"
    rm "$CONFIG_DIR/hub${i}-cluster${j}"
  done
done
rm "$KUBECONFIG"

# ps -ef | grep "e2e" | grep -v grep |awk '{print $2}' | xargs kill -9 >/dev/null 2>&1