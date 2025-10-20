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
CLUSTER_NAME="${CLUSTER_NAME:-global-hub}"
export KUBECONFIG="${CONFIG_DIR}/${CLUSTER_NAME}"

COMPONENT="${COMPONENT:-multicluster-global-hub-operator}"
NAMESPACE="${NAMESPACE:-multicluster-global-hub}"

# If cluster is global-hub, get the actual namespace from multiclusterglobalhub resource(for the BYO case)
if [ "$CLUSTER_NAME" = "global-hub" ]; then
  MGH_NAMESPACE=$(kubectl get multiclusterglobalhub --all-namespaces -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null || echo "")
  if [ -n "$MGH_NAMESPACE" ]; then
    NAMESPACE="$MGH_NAMESPACE"
  fi
fi

echo ">> COMPONENT=$COMPONENT NAMESPACE=$NAMESPACE CLUSTER=$CLUSTER_NAME"

# If component is operator, print MGH and other resources first, then describe and logs
if [ "$COMPONENT" = "multicluster-global-hub-operator" ]; then
  echo ">>>> multiclusterglobalhub"
  kubectl get multiclusterglobalhub -n "$NAMESPACE" -oyaml

  echo ">>>> deploy"
  kubectl get deploy -n "$NAMESPACE"

  echo ">>>> pod"
  kubectl get pod -n "$NAMESPACE"

  echo ">>>> KafkaCluster"
  kubectl get kafka -n "$NAMESPACE" -oyaml 2>/dev/null || echo "Kafka CRD not found or no resources, skipping..."

  echo ">>>> KafkaUsers"
  kubectl get kafkauser -n "$NAMESPACE" -oyaml 2>/dev/null || echo "KafkaUser CRD not found or no resources, skipping..."

  echo ">>>> KafkaTopics"
  kubectl get kafkatopics -n "$NAMESPACE" -oyaml 2>/dev/null || echo "KafkaTopic CRD not found or no resources, skipping..."
fi

# Print describe and logs at the end
kubectl describe deploy "$COMPONENT" -n "$NAMESPACE"
kubectl logs deployment/"$COMPONENT" -n "$NAMESPACE" --all-containers=true
