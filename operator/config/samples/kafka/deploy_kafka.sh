#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# install community kafka operator
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
KAFKA_OPERATOR=${KAFKA_OPERATOR:-"strimzi-cluster-operator-v0.23.0"}
kubectl apply -f ${currentDir}/kafka-operator.yaml

# wait until operator is ready
operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
SECOND=0
while [ -z "$operatorDeployed" ]; do
  if [ $SECOND -gt 600 ]; then
    echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
    exit 1
  fi
  echo "Waiting for strimzi-cluster-operator to become available"
  sleep 10
  (( SECOND = SECOND + 10 ))
  operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
done
kubectl -n kafka wait --for=condition=Available Deployment/$KAFKA_OPERATOR --timeout=600s
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
certificate=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' | base64 -w 0)

kubectl create secret generic kafka-secret -n "open-cluster-management" \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-literal=CA=$certificate
echo "Kafka secret is ready!"