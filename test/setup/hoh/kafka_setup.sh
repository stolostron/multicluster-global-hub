
#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "$0" )" && pwd )"
setupDir="$(cd "$(dirname "$0")/.." ; pwd -P)"
source "$setupDir/common.sh"

# check the transport secret
transportSecret=${TRANSPORT_SECRET_NAME:-"transport-secret"}
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
ready=$(kubectl get secret $transportSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "transportSecret $transportSecret already exists in $targetNamespace namespace"
  exit 0
fi

# deploy kafka operator
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ${currentDir}/components/kafka-community-operator.yaml
waitAppear "kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true"
echo "Kafka operator is ready"

# deploy kafka cluster
kubectl apply -f ${currentDir}/components/kafka-community-cluster.yaml
waitAppear "kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found"
echo "Kafka cluster is ready"

waitAppear "kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec || true"
waitAppear "kubectl get kafkatopic status -n kafka --ignore-not-found | grep status || true"
echo "Kafka topics spec and status are ready!"

# generate transport secret
bootstrapServers=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' > $setupDir/config/kafka-cert.pem
kubectl create secret generic $transportSecret -n $targetNamespace \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$setupDir/config/kafka-cert.pem
echo "transport secret is ready!"


