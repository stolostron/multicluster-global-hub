
#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
setupDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." ; pwd -P)"
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
waitAppear "kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true" 1200
echo "Kafka operator is ready"

# deploy kafka cluster
kubectl apply -f ${currentDir}/components/kafka-community-cluster.yaml
waitAppear "kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found" 1200
echo "Kafka cluster is ready"

waitAppear "kubectl get kafkatopic spec -n kafka --ignore-not-found | grep spec || true"
waitAppear "kubectl get kafkatopic status -n kafka --ignore-not-found | grep status || true"
echo "Kafka topics spec and status are ready!"

# kafkaUser=global-hub-kafka-user
# waitAppear "kubectl get secret ${kafkaUser} -n kafka --ignore-not-found"
# echo "Kafka user ${kafkaUser} is ready!"

# generate transport secret
bootstrapServers=$(kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' > $setupDir/config/kafka-ca-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.crt}' | base64 -d > $setupDir/config/kafka-client-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.key}' | base64 -d > $setupDir/config/kafka-client-key.pem

kubectl create secret generic $transportSecret -n $targetNamespace \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$setupDir/config/kafka-ca-cert.pem
    # --from-file=client.crt=$setupDir/config/kafka-client-cert.pem \
    # --from-file=client.key=$setupDir/config/kafka-client-key.pem 
echo "transport secret is ready!"


