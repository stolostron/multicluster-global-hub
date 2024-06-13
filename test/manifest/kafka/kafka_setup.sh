#!/bin/bash

export KUBECONFIG=${1:-$KUBECONFIG}

current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
setup_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." || exit ; pwd -P)"
# shellcheck source=/dev/null
source "$setup_dir/util.sh"

# check the transport secret
transport_secret=${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"}
target_namespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
kubectl create namespace "$target_namespace" --dry-run=client -o yaml | kubectl apply -f -

if [ -n "$(kubectl get secret "$transport_secret" -n "$target_namespace" --ignore-not-found=true)" ]; then
  echo "transport_secret $transport_secret already exists in $target_namespace namespace"
  exit 0
fi

# deploy kafka operator
retry "(kubectl apply -k $current_dir/kafka-operator -n $target_namespace) && (kubectl get pods -n $target_namespace -l name=strimzi-cluster-operator | grep Running)" 60
echo "Kafka operator is ready"

# deploy kafka cluster
retry "(kubectl apply -k $current_dir/kafka-cluster -n $target_namespace) && (kubectl get kafka kafka -n $target_namespace -o json | jq '(.status.listeners | length) == 2'  | grep true)" 120

# patch the nodeport IP to the broker certificate Subject Alternative Name(SAN)
node_port_host=$(kubectl -n "$target_namespace" get kafka.kafka.strimzi.io/kafka -o jsonpath='{.status.listeners[1].addresses[0].host}')
kubectl -n "$target_namespace" patch kafka.kafka.strimzi.io/kafka --type json -p '[
  {
    "op": "replace",
    "path": "/spec/kafka/listeners/1/configuration",
    "value": {
      "bootstrap": {
        "nodePort": 30095
      },
      "brokers": [
        {
          "broker": 0,
          "advertisedHost": "'"$node_port_host"'", 
        }
      ]
    }
  }
]'

# kafka
wait_cmd "kubectl get kafkatopic event -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkatopic spec -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkatopic status.hub1 -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkatopic status.hub2 -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkauser global-hub-kafka-user -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkauser hub1-kafka-user -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkauser hub2-kafka-user -n multicluster-global-hub | grep -C 1 True"

echo "Kafka cluster is ready"

# BYO: 1. create the topics; 2. create the user; 3. create the transport secret 
# wait_cmd "kubectl get kafkatopic spec -n $target_namespace --ignore-not-found | grep spec || true"
# wait_cmd "kubectl get kafkatopic status -n $target_namespace --ignore-not-found | grep status || true"
# echo "Kafka topics spec and status are ready!"

# kafkaUser=global-hub-kafka-user
# wait_cmd "kubectl get secret ${kafkaUser} -n kafka --ignore-not-found"
# echo "Kafka user ${kafkaUser} is ready!"

## generate transport secret
# bootstrapServers=$(kubectl get kafka kafka -n $target_namespace -o jsonpath='{.status.listeners[1].bootstrapServers}')
# kubectl get kafka kafka -n $target_namespace -o jsonpath='{.status.listeners[1].certificates[0]}' > $setup_dir/config/kafka-ca-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.crt}' | base64 -d > $setup_dir/config/kafka-client-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.key}' | base64 -d > $setup_dir/config/kafka-client-key.pem

## create target namespace
# kubectl create namespace $target_namespace --dry-run=client -o yaml | kubectl apply -f -
# Note: skip to create the transport secret, trying to use the the internal multi users and topics for managed hubs 
# kubectl create secret generic $transport_secret -n $target_namespace \
#     --from-literal=bootstrap_server=$bootstrapServers \
#     --from-file=ca.crt=$setup_dir/config/kafka-ca-cert.pem
#     # --from-file=client.crt=$setup_dir/config/kafka-client-cert.pem \
#     # --from-file=client.key=$setup_dir/config/kafka-client-key.pem 
# echo "transport secret is ready!"


