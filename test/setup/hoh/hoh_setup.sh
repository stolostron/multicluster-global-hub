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

if [[ $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF =~ "@" ]]; then IFS='@'; else IFS=':'; fi
read -ra MGHManagerImageRefArray <<< "$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF"
MGHManagerImageRepo=${MGHManagerImageRefArray[0]}
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REPO=${MGHManagerImageRepo%/*}
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_NAME=${MGHManagerImageRepo##*/}
if [[ $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF =~ "@" ]]; then 
  export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_GIGEST=${MGHManagerImageRefArray[1]}
else
  export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_TAG=${MGHManagerImageRefArray[1]}
fi

if [[ $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF =~ "@" ]]; then IFS='@'; else IFS=':'; fi
read -ra MGHAgentImageRefArray <<< "$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF"
MGHAgentImageRepo=${MGHAgentImageRefArray[0]}
export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REPO=${MGHAgentImageRepo%/*}
export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_NAME=${MGHAgentImageRepo##*/}
if [[ $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF =~ "@" ]]; then 
  export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_GIGEST=${MGHAgentImageRefArray[1]}
else
  export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_TAG=${MGHAgentImageRefArray[1]}
fi

if [[ $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF =~ "@" ]]; then IFS='@'; else IFS=':'; fi
read -ra MGHOperatorImageRefArray <<< "$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF"
MGHOperatorImageRepo=${MGHOperatorImageRefArray[0]}
export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REPO=${MGHOperatorImageRepo%/*}
export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_NAME=${MGHOperatorImageRepo##*/}
if [[ $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF =~ "@" ]]; then 
  export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_GIGEST=${MGHOperatorImageRefArray[1]}
else
  export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_TAG=${MGHOperatorImageRefArray[1]}
fi

namespace=open-cluster-management
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "$0")/../.." ; pwd -P)"

cd ${rootDir}
export IMG=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF
make deploy-operator 
kubectl wait deployment -n "$namespace" multicluster-global-hub-operator --for condition=Available=True --timeout=600s
echo "HoH operator is ready!"

# patch hub-of-hubs-operator images
kubectl patch deployment governance-policy-propagator -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0"}]}}}}'
kubectl patch deployment multicluster-operators-placementrule -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"multicluster-operators-placementrule","image":"quay.io/open-cluster-management-hub-of-hubs/multicloud-operators-subscription:v0.5.0"}]}}}}'
kubectl patch clustermanager cluster-manager --type merge -p '{"spec":{"placementImagePullSpec":"quay.io/open-cluster-management-hub-of-hubs/placement:v0.5.0@sha256:04ba75acdbe4c5b480013f04f5824f828683bc44e9acd6a17600504afc9b6bbe"}}'
echo "HoH images is updated!"

export TRANSPORT_SECRET_NAME="transport-secret"
export STORAGE_SECRET_NAME="storage-secret"
envsubst < ${currentDir}/components/mgh-v1alpha1-cr.yaml | kubectl apply -f - -n "$namespace"
echo "HoH CR is ready!"

kubectl apply -f ${currentDir}/components/manager-service-local.yaml -n "$namespace"
echo "HoH manager nodeport service is ready!"

sleep 2
echo "HoH cr and configmap information:"
kubectl get cm mgh-images-config -n "$namespace" -oyaml 
kubectl get mgh multiclusterglobalhub -n "$namespace" -oyaml