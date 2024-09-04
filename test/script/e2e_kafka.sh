#!/bin/bash

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

KUBECONFIG=${1:-$KUBECONFIG}        # install the kafka
SECRET_KUBECONFIG=${2:-$KUBECONFIG} # generate the crenditial secret

start_time=$(date +%s)
echo -e "\r${BOLD_GREEN}[ START - $(date +"%T") ] Install Kafka $NC"

# check the transport secret
transport_secret=${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"}
target_namespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
if kubectl get secret "$transport_secret" -n "$target_namespace" --kubeconfig "$SECRET_KUBECONFIG"; then
  echo "transport_secret $transport_secret already exists in $target_namespace namespace"
  exit 0
fi

# create all the resource in cluster KUBECONFIG
kubectl create namespace "$target_namespace" --dry-run=client -o yaml | kubectl apply -f -

# deploy kafka operator
kubectl -n $target_namespace create -f "https://strimzi.io/install/latest?namespace=$target_namespace"
retry "(kubectl get pods -n $target_namespace -l name=strimzi-cluster-operator | grep Running)" 60

echo "Kafka operator is ready"

node_port_host=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -e 's#^https\?://##' -e 's/:.*//')
sed -i -e "s;NODE_PORT_HOST;$node_port_host;" "$TEST_DIR"/manifest/kafka/kafka-cluster/kafka-cluster.yaml
# deploy kafka cluster
kubectl apply -k "$TEST_DIR"/manifest/kafka/kafka-cluster -n "$target_namespace"

wait_cmd "kubectl get kafka kafka -n $target_namespace -o jsonpath='{.status.listeners[1]}' | grep bootstrapServers"

# kafka
wait_cmd "kubectl get kafkatopic gh-spec -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkatopic gh-status.hub1 -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkatopic gh-status.hub2 -n multicluster-global-hub | grep -C 1 True"
wait_cmd "kubectl get kafkauser global-hub-kafka-user -n multicluster-global-hub | grep -C 1 True"
echo "Kafka topic/user are ready"

# Note: skip to create the transport secret, trying to use the the internal multi users and topics for managed hubs

# BYO: 1. create the topics; 2. create the user; 3. create the transport secret
byo_user=global-hub-byo-user
wait_cmd "kubectl get kafkauser $byo_user -n $target_namespace | grep -C 1 True"

# generate transport secret
wait_cmd "kubectl get kafka kafka -n $target_namespace -o jsonpath='{.status.listeners[1]}' | grep bootstrapServers"
bootstrap_server=$(kubectl get kafka kafka -n "$target_namespace" -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka -n "$target_namespace" -o jsonpath='{.status.listeners[1].certificates[0]}' >"$CURRENT_DIR"/config/kafka-ca-cert.pem
kubectl get secret $byo_user -n "$target_namespace" -o jsonpath='{.data.user\.crt}' | base64 -d >"$CURRENT_DIR"/config/kafka-client-cert.pem
kubectl get secret $byo_user -n "$target_namespace" -o jsonpath='{.data.user\.key}' | base64 -d >"$CURRENT_DIR"/config/kafka-client-key.pem

# generate the secret in the target cluster: SECRET_KUBECONFIG

kubectl create ns "$target_namespace" --dry-run=client -oyaml | kubectl --kubeconfig "$SECRET_KUBECONFIG" apply -f -
kubectl create secret generic "$transport_secret" -n "$target_namespace" --kubeconfig "$SECRET_KUBECONFIG" \
  --from-literal=bootstrap_server="$bootstrap_server" \
  --from-file=ca.crt="$CURRENT_DIR"/config/kafka-ca-cert.pem \
  --from-file=client.crt="$CURRENT_DIR"/config/kafka-client-cert.pem \
  --from-file=client.key="$CURRENT_DIR"/config/kafka-client-key.pem
echo "transport secret is ready!"

echo -e "\r${BOLD_GREEN}[ END - $(date +"%T") ] Install Kafka ${NC} $(($(date +%s) - start_time)) seconds"
