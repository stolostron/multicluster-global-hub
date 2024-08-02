#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)

# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"
[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

# Set default values if not already set
export CLUSTER_NAME="${CLUSTER_NAME:-global-hub}"
export COMPONENT="${COMPONENT:-multicluster-global-hub-operator}"
export NAMESPACE="${NAMESPACE:-multicluster-global-hub}"

export KUBECONFIG="${CONFIG_DIR}/${CLUSTER_NAME}"

echo ">> COMPONENT=$COMPONENT NAMESPACE=$NAMESPACE CLUSTER=$CLUSTER_NAME"

kubectl logs deployment/"$COMPONENT" -n "$NAMESPACE" --all-containers=true