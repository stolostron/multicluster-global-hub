#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

# setup kubeconfig
KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}
HUB_INIT=${HUB_INIT:-true}

hub="$1"
spoken="$2"
start_time=$(date +%s)

echo -e "\r${BOLD_GREEN}[ START - $(date +"%T") ] $hub : $spoken $NC"
set +e

# init clusters
[ "$HUB_INIT" = true ] && (kind_cluster "$hub") && (init_hub "$hub")
kind_cluster "$spoken"

install_crds "$hub"  # router, mch(not needed for the managed clusters)
install_crds "$spoken" 

retry "(join_cluster $hub $spoken) && kubectl get mcl $spoken --context $hub" 5

# async
init_app "$hub" "$spoken" 
init_policy "$hub" "$spoken" 

enable_cluster "$hub" "$spoken" 

wait_ocm "$hub" "$spoken"
wait_policy "$hub" "$spoken"
wait_application "$hub" "$spoken"

echo -e "\r${BOLD_GREEN}[ END - $(date +"%T") ] $hub : $spoken ${NC} $(($(date +%s) - start_time)) seconds"