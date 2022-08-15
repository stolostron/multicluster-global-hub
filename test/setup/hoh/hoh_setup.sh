# !/bin/bash

set -euo pipefail

branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi
export OPENSHIFT_CI=${OPENSHIFT_CI:-"false"}
export MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF=${MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF:-"quay.io/stolostron/hub-of-hubs-manager:$TAG"}
export MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF=${MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF:-"quay.io/stolostron/hub-of-hubs-agent:$TAG"}
export MULTICLUSTER_GLOBALHUB_OPERATOR_IMAGE_REF=${MULTICLUSTER_GLOBALHUB_OPERATOR_IMAGE_REF:-"quay.io/stolostron/hub-of-hubs-operator:$TAG"}

echo "KUBECONFIG $KUBECONFIG"
echo "OPENSHIFT_CI: $OPENSHIFT_CI"
echo "MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF $MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF"
echo "MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF $MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF"
echo "MULTICLUSTER_GLOBALHUB_OPERATOR_IMAGE_REF $MULTICLUSTER_GLOBALHUB_OPERATOR_IMAGE_REF"

namespace=open-cluster-management
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "$0")/../.." ; pwd -P)"

cd ${rootDir}
make deploy-operator IMG=$MULTICLUSTER_GLOBALHUB_OPERATOR_IMAGE_REF
kubectl wait deployment -n "$namespace" hub-of-hubs-operator --for condition=Available=True --timeout=600s
echo "HoH operator is ready!"

# patch hub-of-hubs-operator images
kubectl patch deployment governance-policy-propagator -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:hub-of-hubs"}]}}}}'
kubectl patch deployment multicluster-operators-placementrule -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"multicluster-operators-placementrule","image":"quay.io/open-cluster-management-hub-of-hubs/multicloud-operators-subscription:hub-of-hubs"}]}}}}'
kubectl patch clustermanager cluster-manager --type merge -p '{"spec":{"placementImagePullSpec":"quay.io/open-cluster-management-hub-of-hubs/placement:hub-of-hubs@sha256:b7293b436dc00506b370762fb4eb352e7c6cc5413d135fc03c93ed311e7ed4c4"}}'
echo "HoH images is updated!"

export KAFKA_SECRET_NAME="kafka-secret"
export POSTGRES_SECRET_NAME="postgresql-secret"
envsubst < ${currentDir}/components/mgh-v1alpha1-cr.yaml | kubectl apply -f - -n "$namespace"
echo "HoH CR is ready!"

kubectl apply -f ${currentDir}/components/manager-service-local.yaml -n "$namespace"
echo "HoH manager nodeport service is ready!"
