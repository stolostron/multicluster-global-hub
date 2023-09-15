#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: check the transport-secret
targetNamespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
transportSecret=${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"}
ready=$(kubectl get secret $transportSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "transportSecret $transportSecret already exists in $targetNamespace namespace"
  exit 0
fi

# step2: deploy kafka operator
kubectl create namespace $targetNamespace --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ${currentDir}/kafka-subscription.yaml
waitAppear "kubectl get pods -n $targetNamespace -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true"
echo "Kafka operator is ready"

# step3: deploy Kafka cluster
kubectl apply -f ${currentDir}/kafka-cluster.yaml
waitAppear "kubectl -n $targetNamespace get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found"
echo "Kafka cluster is ready"

# step4: deploy Kafka topics
kubectl apply -f ${currentDir}/kafka-topics.yaml
waitAppear "kubectl get kafkatopic spec -n $targetNamespace --ignore-not-found | grep spec || true"
waitAppear "kubectl get kafkatopic status -n $targetNamespace --ignore-not-found | grep status || true"
echo "Kafka topics spec and status are ready!"

# step5: deploy Kafka user to generate tls secret
kafkaUser=global-hub-kafka-user
kubectl apply -f ${currentDir}/kafka-user.yaml
waitAppear "kubectl get secret ${kafkaUser} -n $targetNamespace --ignore-not-found"

# step5: generate transport-secret
bootstrapServers=$(kubectl get kafka kafka -n $targetNamespace -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka -n $targetNamespace -o jsonpath='{.status.listeners[1].certificates[0]}' > $currentDir/kafka-ca-cert.pem
# kubectl get secret kafka-clients-ca-cert -n $targetNamespace -o jsonpath='{.data.ca\.crt}' | base64 -d > $currentDir/kafka-ca-cert.pem
kubectl get secret ${kafkaUser} -n $targetNamespace -o jsonpath='{.data.user\.crt}' | base64 -d > $currentDir/kafka-client-cert.pem
kubectl get secret ${kafkaUser} -n $targetNamespace -o jsonpath='{.data.user\.key}' | base64 -d > $currentDir/kafka-client-key.pem

kubectl create namespace $targetNamespace || true
kubectl create secret generic ${transportSecret} -n $targetNamespace \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$currentDir/kafka-ca-cert.pem \
    --from-file=client.crt=$currentDir/kafka-client-cert.pem \
    --from-file=client.key=$currentDir/kafka-client-key.pem 

rm $currentDir/kafka-ca-cert.pem
rm $currentDir/kafka-client-cert.pem
rm $currentDir/kafka-client-key.pem
echo "transport secret is ready in $targetNamespace namespace!"