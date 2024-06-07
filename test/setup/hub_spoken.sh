#!/bin/bash

set -euox pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

# setup kubeconfig
export KUBE_DIR=${KUBE_DIR:${CURRENT_DIR}/kubeconfig} 
check_dir "$KUBE_DIR"
export KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/kind-clusters}

hub="$1"
spoken="$2"
skip_hub_init=${3:-false}

# init clusters
echo -e "$BLUE $hub - $spoken: creating clusters  $NC"
kind_cluster "$hub"
kind_cluster "$spoken"

echo -e "$BLUE $hub - $spoken: initializing $hub $NC"
[ "$skip_hub_init" = false ] && (init_hub "$hub")

# resources
node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$hub-control-plane")
kubectl --kubeconfig "$KUBE_DIR/kind-$hub" config set-cluster "kind-$hub" --server="https://$node_ip:6443"

install_crds "$hub"  # router, mch(not needed for the managed clusters)
install_crds "$spoken" 

echo -e "$BLUE $hub - $spoken: importing $spoken $NC"
join_cluster "$hub" "$spoken" 

# async
echo -e "$BLUE $hub - $spoken: deploying app and policy $NC"
init_app "$hub" "$spoken" 
init_policy "$hub" "$spoken" 

enable_cluster "$hub" "$spoken" 