#!/bin/bash
# Source this script to enable the kafka cluster for hub-of-hubs. The olm component must be install before exec this script.

function deployKafka() {
  # while the namespace is not ready, wait for it
  echo "Creating kafka namespace"
  kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
  # SECOND=0
  # while [[ -z "$(kubectl get namespace kafka --ignore-not-found=true)" ]]; do
  #   if [ $SECOND -gt 300 ]; then
  #     echo "Timeout waiting for namespace kafka"
  #     exit 1
  #   fi
    
  #   sleep 5
  #   (( SECOND = SECOND + 5 ))
  # done

  # install community kafka operator
  KAFKA_OPERATOR=${KAFKA_OPERATOR:-"strimzi-cluster-operator-v0.23.0"}
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  kubectl apply -f ${currentDir}/kafka-operator.yaml
    
  # wait until operator is ready
  operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
  SECOND=0
  while [ -z "$operatorDeployed" ]; do
    if [ $SECOND -gt 400 ]; then
      echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
      exit 1
    fi
    echo "Waiting for strimzi-cluster-operator to become available"
    sleep 20
    (( SECOND = SECOND + 20 ))
    operatorDeployed=$(kubectl -n kafka get Deployment/$KAFKA_OPERATOR --ignore-not-found)
  done
  kubectl -n kafka wait --for=condition=Available Deployment/$KAFKA_OPERATOR --timeout=600s

  # deploy Kafka cluster CR
  kubectl apply -f ${currentDir}/kafka-cluster.yaml
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  SECOND=0
  while [ -z "$clusterIsReady" ]; do
    if [ $SECOND -gt 400 ]; then
      echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
      exit 1
    fi
    echo "Waiting for kafka cluster to become available"
    sleep 30
    (( SECOND = SECOND + 30 ))
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
}

deployKafka