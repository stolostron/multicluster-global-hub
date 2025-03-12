#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

# whether delete the kind clusters
DELETE=${DELETE:-true} 

# setup kubeconfig
KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}

# stop qe test container
docker stop qe-test
docker rm qe-test

echo "kill process"
while read -r line; do
  if [[ $line != "" ]]; then
    kill -9 "${line}" >/dev/null 2>&1 
  fi
done <"$CONFIG_DIR/PID"

ps -ef | grep "ocm.sh" | grep -v grep |awk '{print $2}' | xargs kill -9 >/dev/null 2>&1
ps -ef | grep -E 'e2e-setup|e2e_setup' | grep -v grep |awk '{print $2}' | xargs kill -9 >/dev/null 2>&1

[ "$DELETE" = false ] && exit 0

echo "delete kind clusters"
kind delete cluster --name "${GH_NAME}" > /dev/null 2>&1
for i in $(seq 1 "${MH_NUM}"); do
  echo "delete hub${i}"
  kind delete cluster --name "hub${i}" > /dev/null 2>&1
  rm "$CONFIG_DIR/hub${i}" > /dev/null 2>&1 
  for j in $(seq 1 "${MC_NUM}"); do
    echo "delete hub${i}-cluster${j}"
    kind delete cluster --name "hub${i}-cluster${j}" > /dev/null 2>&1
    rm "$CONFIG_DIR/hub${i}-cluster${j}" > /dev/null 2>&1
  done
done

echo "delete config $CONFIG_DIR"
rm -rf $CONFIG_DIR/*

