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
echo ">> ISBYO= $ISBYO"

kubectl describe deploy "$COMPONENT" -n "$NAMESPACE"
kubectl logs deployment/"$COMPONENT" -n "$NAMESPACE" --all-containers=true

[ "$COMPONENT" != "multicluster-global-hub-operator" ] && exit 0

echo ">>>> KafkaCluster"
kubectl get kafka -n "$NAMESPACE" -oyaml

echo ">>>> KafkaUsers"
kubectl get kafkauser -n "$NAMESPACE" -oyaml

echo ">>>> KafkaTopics"
kubectl get kafkatopics -n "$NAMESPACE" -oyaml

echo ">>>> pod"
kubectl get pod -n "$NAMESPACE"

echo ">>>> deploy"
kubectl get deploy -n "$NAMESPACE"

echo ">>>> mgh"
kubectl get mgh -n "$NAMESPACE" -oyaml
