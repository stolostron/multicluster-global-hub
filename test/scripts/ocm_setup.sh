#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)

# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

# setup kubeconfig
CONFIG_DIR=${CONFIG_DIR:-${CURRENT_DIR}/config} 
KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}
HUB_INIT=${HUB_INIT:-true}

check_dir "$CONFIG_DIR"

hub="$1"
spoken="$2"
start_time=$(date +%s)

echo -e "\r${BOLD_GREEN}[ START ] $hub : $spoken $NC"
set +e

# init clusters
[ "$HUB_INIT" = true ] && (kind_cluster "$hub") && (init_hub "$hub")
kind_cluster "$spoken"

install_crds "$hub"  # router, mch(not needed for the managed clusters)
install_crds "$spoken" 

retry "(join_cluster $hub $spoken) && kubectl get mcl $spoken --context $hub" 10

# async
init_app "$hub" "$spoken" 
init_policy "$hub" "$spoken" 

enable_cluster "$hub" "$spoken" 

echo -e "\r${BOLD_GREEN}[ END ] $hub : $spoken ${NC} $(($(date +%s) - start_time)) seconds"