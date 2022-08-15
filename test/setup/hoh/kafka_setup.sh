
#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

kafkaSecret=${KAFKA_SECRET_NAME:-"kafka-secret"}
ready=$(kubectl get secret $kafkaSecret -n open-cluster-management --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "kafkaSecret $kafkaSecret already exists in open-cluster-management namespace"
  exit 0
fi
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ${currentDir}/components/kafka-community-operator.yaml
  
# wait until operator is ready
operatorName=${operatorName:-"strimzi-cluster-operator-v0.23.0"}
operatorDeployed=$(kubectl -n kafka get Deployment/$operatorName --ignore-not-found)
SECOND=0
while [ -z "$operatorDeployed" ]; do
  if [ $SECOND -gt 600 ]; then
    echo "Timeout waiting for deploying strimzi-cluster-operator $operatorDeployed"
    exit 1
  fi
  echo "Waiting for strimzi-cluster-operator to become available"
  sleep 10
  (( SECOND = SECOND + 10 ))
  operatorDeployed=$(kubectl -n kafka get Deployment/$operatorName --ignore-not-found)
done
kubectl -n kafka wait --for=condition=Available Deployment/$operatorName --timeout=600s

kubectl apply -f ${currentDir}/components/kafka-community-cluster.yaml
# wati Kafka cluster CR
clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
SECOND=0
while [ -z "$clusterIsReady" ]; do
  if [ $SECOND -gt 600 ]; then
    echo "Timeout waiting for deploying kafka.kafka.strimzi.io/kafka-brokers-cluster $operatorDeployed"
    exit 1
  fi
  echo "Waiting for kafka cluster to become available"
  sleep 10
  (( SECOND = SECOND + 10 ))
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
done
echo "Kafka cluster is ready"

isSpecReady=$(kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec)
SECOND=0
while [[ -z "$isSpecReady" ]]; do
  echo "Waiting for kafka topics to become available"
  sleep 5
  (( SECOND = SECOND + 5 ))
  isSpecReady=$(kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec)
done
echo "Kafka topics are ready!"

setupDir="$(cd "$(dirname "$0")/.." ; pwd -P)"
bootstrapServers=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' > $setupDir/config/kafka-cert.pem
kubectl create secret generic $kafkaSecret -n "open-cluster-management" \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=CA=$setupDir/config/kafka-cert.pem
echo "Kafka secret is ready!"


