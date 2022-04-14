#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

# create namespace if not exists
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -

# deploy AMQ streams operator
curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-kafka-transport/$TAG/deploy/amq_streams_operator.yaml" | kubectl apply -f -

# wait until operator is ready
operator_deployed=$(kubectl -n kafka get Deployment/amq-streams-cluster-operator-v1.8.4 --ignore-not-found)
while [ -z "$operator_deployed" ]; do
    echo "Waiting for AMQ streams operator to become available"
    sleep 10
    operator_deployed=$(kubectl -n kafka get Deployment/amq-streams-cluster-operator-v1.8.4 --ignore-not-found)
done
kubectl -n kafka wait --for=condition=Available Deployment/amq-streams-cluster-operator-v1.8.4 --timeout=600s

# deploy Kafka cluster CR
curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-kafka-transport/$TAG/deploy/kafka-cluster.yaml" | kubectl apply -f -

# wait until kafka cluster is ready
cluster_is_ready=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
while [ -z "$cluster_is_ready" ]; do
    echo "Waiting for kafka cluster to become available"
    sleep 60
    cluster_is_ready=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
done

echo "kafka cluster is ready"
