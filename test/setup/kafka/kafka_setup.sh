#!/bin/bash
# Source this script to enable the kafka cluster for hub-of-hubs. The olm component must be install before exec this script.

function deployKafka() {
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  if [[ ! -z "$clusterIsReady" ]]; then 
    echo "Kafka cluster already exists!"
    exit 0
  fi 

  # create namespace if not exists
  kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -

  # install community kafka operator
  KAFKA_OPERATOR=${KAFKA_OPERATOR:-"strimzi-cluster-operator-v0.23.0"}
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  kubectl apply -f ${currentDir}/kafka-operator.yaml
    
  # wait until operator is ready
  operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
  while [ -z "$operatorDeployed" ]; do
      echo "Waiting for strimzi-cluster-operator to become available"
      sleep 20
      operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
  done
  kubectl -n kafka wait --for=condition=Available Deployment/$KAFKA_OPERATOR --timeout=600s

  # deploy Kafka cluster CR
  kubectl apply -f ${currentDir}/kafka-cluster.yaml
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  while [ -z "$clusterIsReady" ]; do
    echo "Waiting for kafka cluster to become available"
    sleep 30
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
}

deployKafka