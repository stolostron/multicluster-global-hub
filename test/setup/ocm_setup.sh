#!/bin/bash

set -euxo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

# setup kubeconfig
KUBE_DIR=${KUBE_DIR:-${CURRENT_DIR}/kubeconfig} 
KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/clusters}
check_dir "$KUBE_DIR"

HUB_INIT=${HUB_INIT:-true}

hub="$1"
spoken="$2"

# init clusters
kind_cluster "$hub"
kind_cluster "$spoken"

[ "$HUB_INIT" = true ] && (init_hub "$hub")

install_crds "$hub"  # router, mch(not needed for the managed clusters)
install_crds "$spoken" 

retry "(join_cluster $hub $spoken) && kubectl get mcl $spoken --context $hub" 10

# async
init_app "$hub" "$spoken" 
init_policy "$hub" "$spoken" 

enable_cluster "$hub" "$spoken" 