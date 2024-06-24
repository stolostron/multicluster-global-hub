#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

export TAG=${TAG:-"latest"}
export OPENSHIFT_CI=${OPENSHIFT_CI:-"false"}
export REGISTRY=${REGISTRY:-"quay.io/stolostron"}

if [[ $OPENSHIFT_CI == "false" ]]; then
  export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-manager:${TAG}"}
  export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-agent:$TAG"}
  export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-operator:$TAG"}
fi

echo "OPENSHIFT_CI: $OPENSHIFT_CI"
echo "MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF"
echo "MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF"
echo "MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF"

# set 
gh_namespace="multicluster-global-hub"
cd test/manifest/resources && (kustomize edit set namespace $gh_namespace) && (cd "${CURRENT_DIR}")
kustomzie build test/manifest/resources | kubectl apply -f -

kustomize edit set namespace $gh_namespace config/default
kustomzie edit set image quay.io/stolostron/multicluster-global-hub-operator=${IMG} config/manager
kustomzie build config/default | kubectl apply -f -