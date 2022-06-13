#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config

KUBECONFIG=${CONFIG_DIR}/kubeconfig

touch "$CONFIG_DIR/pid_hoh"
while read -r line; do
  if [[ $line != "" ]]; then
    kill -9 "${line}" >/dev/null 2>&1
  fi
done <"${CONFIG_DIR}/pid_hoh"

rm "${CONFIG_DIR}/pid_hoh" >/dev/null 2>&1

docker stop acm-hub-of-hubs
docker volume prune -f 
docker rmi quay.io/microshift/microshift-aio:latest

wget -O - https://github.com/stolostron/hub-of-hubs/blob/release-2.5/demos/setup/ocm_clean.sh | bash

rm -rf $CONFIG_DIR