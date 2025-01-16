#!/bin/bash

global_hub_namespace=${2:-"multicluster-global-hub"}
kakfa_cluster_name=${3:-"kafka"}

# create a KafkaUser CR
createKafkaUser() {
  cat <<EOF | kubectl apply -f -
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaUser
metadata:
  labels:
    strimzi.io/cluster: $kakfa_cluster_name
  name: $1-kafka-user
  namespace: $global_hub_namespace
spec:
  authentication:
    type: tls
  authorization:
    acls:
    - host: '*'
      operations:
      - Read
      resource:
        name: '*'
        patternType: literal
        type: group
    - host: '*'
      operations:
      - Describe
      - Read
      resource:
        name: gh-spec
        patternType: literal
        type: topic
    - host: '*'
      operations:
      - Write
      resource:
        name: gh-status.$1
        patternType: literal
        type: topic
    type: simple
EOF
}

# create a KafkaTopic CR
createKafkaTopic() {
  cat <<EOF | kubectl apply -f -
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  labels:
    strimzi.io/cluster: $kakfa_cluster_name
  name: gh-status.$1
  namespace: $global_hub_namespace
spec:
  config:
    cleanup.policy: compact
  partitions: 1
  replicas: 3
EOF
}

# generate kafka.yaml file
generateKafkaConfig() {

  end=$((SECONDS + 60))
  while ((SECONDS < end)); do
    if kubectl get secret "$1-kafka-user" -n $global_hub_namespace >/dev/null 2>&1; then
      echo "Secret $1-kafka-user is ready in namespace $global_hub_namespace"
      break
    fi
    echo "Waiting for secret $1-kafka-user..."
    sleep 1
  done
  if ((SECONDS >= end)); then
    echo "Timeout waiting for secret $1-kafka-user"
    exit 1
  fi
  cat <<EOF >kafka.yaml
bootstrap.server: $(kubectl get kafka kafka -n $global_hub_namespace -o jsonpath='{.status.listeners[0].bootstrapServers}')
topic.status: gh-status.$1
ca.crt: $(kubectl get kafka kafka -n $global_hub_namespace -o jsonpath='{.status.listeners[0].certificates[0]}' | { if [[ "$OSTYPE" == "darwin"* ]]; then base64 -b 0; else base64 -w 0; fi; })
client.crt: $(kubectl get secret $1-kafka-user -n $global_hub_namespace -o jsonpath='{.data.user\.crt}')
client.key: $(kubectl get secret $1-kafka-user -n $global_hub_namespace -o jsonpath='{.data.user\.key}')
EOF
  # print the generated kafka.yaml file
  echo ""
  echo "==========================================="
  echo "Configuration Instructions"
  echo "==========================================="
  echo ""
  echo "1. Copy the following content into your local kafka.yaml file."
  echo ""
  echo "2. Use the following command to create the transport-config secret in the 'multicluster-global-hub' namespace:"
  echo ""
  echo "   kubectl create secret generic transport-config \\"
  echo "       -n multicluster-global-hub \\"
  echo "       --from-file=kafka.yaml=./kafka.yaml"
  echo ""
  echo "==========================================="
  echo "kafka.yaml Content:"
  echo "==========================================="
  echo ""
  cat kafka.yaml
  echo ""
}

# create a KafkaUser and KafkaTopic for the given name
createKafkaTopic $1
if [ $? -ne 0 ]; then
    exit 1
fi

createKafkaUser $1
if [ $? -ne 0 ]; then
    exit 1
fi

generateKafkaConfig $1
if [ $? -ne 0 ]; then
    exit 1
fi
