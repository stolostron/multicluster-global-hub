#!/bin/bash

set -e

global_hub_namespace=${2:-"multicluster-global-hub"}
kakfa_cluster_name=${3:-"kafka"}

usage() {
  cat <<EOF
Usage: $(basename "${BASH_SOURCE[0]}") cluster_name [global_hub_namespace] [kafka_cluster_name]

Create a KafkaTopic and KafkaUser in your cluster.
Generate the Kafka configurations, including bootstrap.server, topic.status, ca.crt, client.crt, and client.key.

Available options:

  -h, --help                 Print this help message and exit
  cluster_name               The OpenShift cluster that is used by the KafkaUser and KafkaTopic.
  global_hub_namespace       The namespace of the Global Hub operator. Default is 'multicluster-global-hub'.
  kafka_cluster_name         The Kafka cluster name. Default is 'kafka'.

EOF
  exit
}

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
    # Retention policy: delete messages when EITHER condition is met
    # - retention.ms: 24 hours (86400000 ms)
    # - retention.bytes: 1GB per partition (1073741824 bytes)
    cleanup.policy: compact,delete
    retention.ms: "86400000"
    retention.bytes: "1073741824"
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

if [ $# -eq 0 -o $# -gt 3 ]; then
  usage
fi

while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -h | --help)
      usage
      ;;

    *)
      break
      ;;
  esac
done
# create a KafkaUser and KafkaTopic for the given name
createKafkaTopic $1
createKafkaUser $1
generateKafkaConfig $1
