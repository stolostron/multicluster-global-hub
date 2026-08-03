#!/bin/bash
set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
source "$CURRENT_DIR/util.sh"

export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/global-hub}

while getopts ":n:" opt; do
  case $opt in
  n)
    GH_NAMESPACE="$OPTARG"
    ;;
  \?)
    echo "Invalid option -$OPTARG" >&2
    exit 1
    ;;
  esac

  case $OPTARG in
  -*)
    echo "Option $opt needs a valid argument"
    exit 1
    ;;
  esac
done

GH_NAMESPACE=${GH_NAMESPACE:=multicluster-global-hub}
echo "namespace: "$GH_NAMESPACE

echo "Delete mgh"
wait_cmd "kubectl delete multiclusterglobalhubs --all -n $GH_NAMESPACE --ignore-not-found=true"

export TARGET_NAMESPACE=$GH_NAMESPACE

## wait kafka/kafkatopic/kafka user be deleted
echo "Check kafkatopics deleted"
if [[ ! -z $(kubectl get kafkatopic -n "$GH_NAMESPACE" --ignore-not-found=true) ]]; then
  echo "Failed to delete kafkatopics"
  exit 1
fi

echo "Check kafkauser deleted"
if [[ ! -z $(kubectl get kafkauser -n "$GH_NAMESPACE" --ignore-not-found=true) ]]; then
  echo "Failed to delete kafkausers"
  exit 1
fi

echo "Check kafka deleted"
if [[ ! -z $(kubectl get kafka -n "$GH_NAMESPACE" --ignore-not-found=true) ]]; then
  echo "Failed to delete kafka"
  exit 1
fi

echo "Delete e2e nonk8s NodePort service"
kubectl delete service multicluster-global-hub-manager-nonk8s-service -n "$GH_NAMESPACE" --ignore-not-found=true
kubectl delete service multicluster-global-hub-manager-nonk8s-service -n mgh --ignore-not-found=true

if [[ "$GH_NAMESPACE" == "multicluster-global-hub" ]]; then
  cd operator
  make undeploy

  ## clean
  wait_cmd "kubectl delete crd kafkas.kafka.strimzi.io --ignore-not-found=true"
  wait_cmd "kubectl delete crd kafkanodepools.kafka.strimzi.io --ignore-not-found=true"
  wait_cmd "kubectl delete crd kafkatopics.kafka.strimzi.io --ignore-not-found=true"
  wait_cmd "kubectl delete crd kafkausers.kafka.strimzi.io --ignore-not-found=true"
else
  # BYO deploys to mgh; make undeploy removes cluster-scoped RBAC/CRDs and can
  # delete leader-election roles in multicluster-global-hub, breaking the next
  # suite that deploys the operator there.
  echo "Delete BYO namespace $GH_NAMESPACE (skip operator undeploy and CRD deletion)"
  kubectl delete namespace "$GH_NAMESPACE" --ignore-not-found=true --timeout=180s
fi

echo "Recreate namespace for subsequent e2e runs"
kubectl get namespace "$GH_NAMESPACE" >/dev/null 2>&1 || kubectl create namespace "$GH_NAMESPACE"

