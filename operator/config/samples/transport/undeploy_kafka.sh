#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: delete transport secret 
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
transportSecret=${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"}
kubectl delete secret ${transportSecret} -n $targetNamespace

# step2: delete kafka topics
kubectl delete -f ${currentDir}/kafka-topics.yaml
waitDisappear "kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec || true"
waitDisappear "kubectl get kafkatopic status -n kafka --ignore-not-found | grep status || true"

# step3: delete kafka cluster
kubectl delete -f ${currentDir}/kafka-cluster.yaml
waitDisappear "kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster --ignore-not-found"

# step4: delete kafka operator
kubectl delete -f ${currentDir}/kafka-operator.yaml
kubectl delete deploy --all -n kafka 
waitDisappear "kubectl get pods -n kafka | grep strimzi-cluster-operator | grep Running || true"

# step5: delete kafka namesapce
kubectl delete namespace kafka 





