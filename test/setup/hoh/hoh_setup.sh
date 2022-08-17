# !/bin/bash

set -euo pipefail

branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi
export OPENSHIFT_CI=${OPENSHIFT_CI:-"false"}
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF:-"quay.io/stolostron/multicluster-global-hub-manager:$TAG"}
export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF:-"quay.io/stolostron/multicluster-global-hub-agent:$TAG"}
export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF:-"quay.io/stolostron/multicluster-global-hub-operator:$TAG"}

echo "KUBECONFIG $KUBECONFIG"
echo "OPENSHIFT_CI: $OPENSHIFT_CI"
echo "MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF"
echo "MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF"
echo "MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF"

namespace=open-cluster-management
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "$0")/../.." ; pwd -P)"

cd ${rootDir}
make deploy-operator IMG=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF
kubectl wait deployment -n "$namespace" multicluster-global-hub-operator --for condition=Available=True --timeout=600s
echo "HoH operator is ready!"

# patch hub-of-hubs-operator images
kubectl patch deployment governance-policy-propagator -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:hub-of-hubs"}]}}}}'
kubectl patch deployment multicluster-operators-placementrule -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"multicluster-operators-placementrule","image":"quay.io/open-cluster-management-hub-of-hubs/multicloud-operators-subscription:hub-of-hubs"}]}}}}'
kubectl patch clustermanager cluster-manager --type merge -p '{"spec":{"placementImagePullSpec":"quay.io/open-cluster-management-hub-of-hubs/placement:hub-of-hubs@sha256:b7293b436dc00506b370762fb4eb352e7c6cc5413d135fc03c93ed311e7ed4c4"}}'
echo "HoH images is updated!"

export TRANSPORT_SECRET_NAME="transport-secret"
export STORAGE_SECRET_NAME="storage-secret"
envsubst < ${currentDir}/components/mgh-v1alpha1-cr.yaml | kubectl apply -f - -n "$namespace"
echo "HoH CR is ready!"

kubectl apply -f ${currentDir}/components/manager-service-local.yaml -n "$namespace"
echo "HoH manager nodeport service is ready!"
