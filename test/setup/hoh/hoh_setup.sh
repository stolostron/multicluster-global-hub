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
agenAddonNamespace=open-cluster-management-global-hub-system
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "$0")/../.." ; pwd -P)"

# create leader election configuration
kubectl apply -f ${currentDir}/components/leader-election-configmap.yaml -n "$namespace"

cd ${rootDir}
# install crds
kubectl --context kind-$LEAF_HUB_NAME apply -f ./pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml

export IMG=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF
make deploy-operator 
kubectl wait deployment -n "$namespace" multicluster-global-hub-operator --for condition=Available=True --timeout=600s
echo "HoH operator is ready!"

export TRANSPORT_SECRET_NAME="transport-secret"
export STORAGE_SECRET_NAME="storage-secret"
envsubst < ${currentDir}/components/mgh-images-config.yaml | kubectl apply -f - -n "$namespace"
envsubst < ${currentDir}/components/mgh-v1alpha2-cr.yaml | kubectl apply -f - -n "$namespace"
echo "HoH CR is ready!"

kubectl patch deployment governance-policy-propagator -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:v0.5.0"}]}}}}'
kubectl patch clustermanager cluster-manager --type merge -p '{"spec":{"placementImagePullSpec":"quay.io/open-cluster-management/placement:latest"}}'
echo "HoH images is updated!"

kubectl apply -f ${currentDir}/components/manager-service-local.yaml -n "$namespace"
echo "HoH manager nodeport service is ready!"

sleep 2
echo "HoH cr and configmap information:"
kubectl get cm mgh-images-config -n "$namespace" -oyaml 
kubectl get mgh multiclusterglobalhub -n "$namespace" -oyaml

# wait for core components to be ready
SECOND=0
while [[ -z $(kubectl get deploy -n $namespace multicluster-global-hub-manager --ignore-not-found) ]]; do
  if [ $SECOND -gt 200 ]; then
    echo "Timeout waiting for deploying multicluster-global-hub-manager in namespace $namespace"
    exit 1
  fi
  echo "Waiting for multicluster-global-hub-manager to be created..."
  sleep 2;
  (( SECOND = SECOND + 2 ))
done;
kubectl wait deployment -n $namespace multicluster-global-hub-manager --for condition=Available=True --timeout=600s

# Need to hack here to fix the microshift issue - https://github.com/openshift/microshift/issues/660
kubectl annotate mutatingwebhookconfiguration multicluster-global-hub-mutator service.beta.openshift.io/inject-cabundle-
ca=$(kubectl get secret multicluster-global-hub-webhook-certs -n $namespace -o jsonpath="{.data.tls\.crt}")
kubectl patch mutatingwebhookconfiguration multicluster-global-hub-mutator -n $namespace -p "{\"webhooks\":[{\"name\":\"global-hub.open-cluster-management.io\",\"clientConfig\":{\"caBundle\":\"$ca\"}}]}"

SECOND=0
while [[ -z $(kubectl get deploy -n $agenAddonNamespace multicluster-global-hub-agent --context kind-$LEAF_HUB_NAME --ignore-not-found) ]]; do
  if [ $SECOND -gt 200 ]; then
    echo "Timeout waiting for deploying multicluster-global-hub-agent in namespace $agenAddonNamespace"
    exit 1
  fi
  echo "Waiting for multicluster-global-hub-agent to be created..."
  sleep 2;
  (( SECOND = SECOND + 2 ))
done;
kubectl --context kind-$LEAF_HUB_NAME wait deployment -n $agenAddonNamespace multicluster-global-hub-agent --for condition=Available=True --timeout=600s
