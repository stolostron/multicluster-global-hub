#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

TARGET_NAMESPACE=${TARGET_NAMESPACE:-"open-cluster-management"}
transportSecret=${TRANSPORT_SECRET_NAME:-"transport-secret"}
ready=$(kubectl get secret $transportSecret -n $TARGET_NAMESPACE --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "transportSecret $transportSecret already exists in $TARGET_NAMESPACE namespace"
  exit 0
fi

# install community kafka operator
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ${currentDir}/kafka-operator.yaml

# wait until operator is ready
operatorDeployed=$(kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true)
SECOND=0
while [ -z "$operatorDeployed" ]; do
  if [ $SECOND -gt 600 ]; then
    echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
    exit 1
  fi
  echo "Waiting for strimzi-cluster-operator to become available"
  sleep 10
  (( SECOND = SECOND + 10 ))
  operatorDeployed=$(kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true)
done
echo "Kafka operator is ready"

# deploy Kafka cluster CR
kubectl apply -f ${currentDir}/kafka-cluster.yaml
clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
SECOND=0
while [ -z "$clusterIsReady" ]; do
  if [ $SECOND -gt 600 ]; then
    echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
    exit 1
  fi
  echo "Waiting for kafka cluster to become available"
  sleep 30
  (( SECOND = SECOND + 30 ))
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
done
echo "Kafka cluster is ready"

# deploy Kafka topic 
kubectl apply -f ${currentDir}/kafka-topics.yaml
isSpecReady=$(kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec)
while [[ -z "$isSpecReady" ]]; do
  sleep 5
  isSpecReady=$(kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec)
done
isStatusReady=$(kubectl get kafkatopic status -n kafka --ignore-not-found | grep status)
while [[ -z "$isStatusReady" ]]; do
  sleep 5
  isStatusReady=$(kubectl get kafkatopic status -n kafka --ignore-not-found | grep status)
done
echo "Kafka topics spec and status are ready!"

bootstrapServers=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' > $currentDir/kafka-cert.pem
kubectl create secret generic ${transportSecret} -n $TARGET_NAMESPACE \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$currentDir/kafka-cert.pem

rm $currentDir/kafka-cert.pem
echo "transport secret is ready in $TARGET_NAMESPACE namespace!"