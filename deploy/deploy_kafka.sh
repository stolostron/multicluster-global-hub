#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"

# create namespace if not exists
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -

# deploy AMQ streams operator
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-kafka-transport/$TAG/deploy/amq_streams_operator.yaml" | kubectl apply -f -

# sleep 5 seconds, wait until operators becomes available
sleep 5

# deploy Kafka cluster CR
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-kafka-transport/$TAG/deploy/kafka-cluster.yaml" | kubectl apply -f -
