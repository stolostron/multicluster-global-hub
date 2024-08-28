#!/bin/bash
set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
source "$CURRENT_DIR/util.sh"

export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/global-hub}

echo "Delete mgh"
wait_cmd "kubectl delete mgh --all -n multicluster-global-hub --ignore-not-found=true"

cd operator
make undeploy

cd $CURRENT_DIR

## wait kafka/kafkatopic/kafka user be deleted
echo "Check kafkatopics deleted"
if [[ ! -z $(kubectl get kafkatopic --ignore-not-found=true) ]]; then
  echo "Failed to delete kafkatopics"
  exit 1
fi

echo "Check kafkauser deleted"
if [[ ! -z $(kubectl get kafkauser --ignore-not-found=true) ]]; then
  echo "Failed to delete kafkausers"
  exit 1
fi

echo "Check kafka deleted"
if [[ ! -z $(kubectl get kafka --ignore-not-found=true) ]]; then
  echo "Failed to delete kafka"
  exit 1
fi

## clean
wait_cmd "kubectl delete crd kafkas.kafka.strimzi.io --ignore-not-found=true"
wait_cmd "kubectl delete crd kafkanodepools.kafka.strimzi.io --ignore-not-found=true"
wait_cmd "kubectl delete crd kafkatopics.kafka.strimzi.io --ignore-not-found=true"
wait_cmd "kubectl delete crd kafkausers.kafka.strimzi.io --ignore-not-found=true"

