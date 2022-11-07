#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "$0" )" && pwd )"
rootDir="$(cd "$(dirname "$0")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: check the transport-secret
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
transportSecret=${TRANSPORT_SECRET_NAME:-"transport-secret"}
ready=$(kubectl get secret $transportSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "transportSecret $transportSecret already exists in $targetNamespace namespace"
  exit 0
fi

# step2: deploy kafka operator
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ${currentDir}/kafka-operator.yaml
waitAppear "kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true"
echo "Kafka operator is ready"

# step3: deploy Kafka cluster
kubectl apply -f ${currentDir}/kafka-cluster.yaml
waitAppear "kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found"
echo "Kafka cluster is ready"

# step4: deploy Kafka topics
kubectl apply -f ${currentDir}/kafka-topics.yaml
waitAppear "kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec || true"
waitAppear "kubectl get kafkatopic status -n kafka --ignore-not-found | grep status || true"
echo "Kafka topics spec and status are ready!"

# step5: generate transport-secret
bootstrapServers=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' > $currentDir/kafka-cert.pem
kubectl create secret generic ${transportSecret} -n $targetNamespace \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$currentDir/kafka-cert.pem

rm $currentDir/kafka-cert.pem
echo "transport secret is ready in $targetNamespace namespace!"