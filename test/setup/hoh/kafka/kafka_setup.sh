
#!/bin/bash

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
setupDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." ; pwd -P)"
source "$setupDir/common.sh"
CTX_HUB=$1

# check the transport secret
transportSecret=${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"}
targetNamespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
kubectl --context $CTX_HUB create namespace $targetNamespace --dry-run=client -o yaml | kubectl apply -f -

ready=$(kubectl --context $CTX_HUB get secret $transportSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "transportSecret $transportSecret already exists in $targetNamespace namespace"
  exit 0
fi

# deploy kafka operator
kubectl --context $CTX_HUB apply -k ${currentDir}/kafka-operator -n $targetNamespace
waitAppear "kubectl --context $CTX_HUB get pods -n $targetNamespace -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true" 1200
echo "Kafka operator is ready"

# deploy kafka cluster
kubectl --context $CTX_HUB apply -k ${currentDir}/kafka-cluster -n $targetNamespace
waitAppear "kubectl --context $CTX_HUB -n $targetNamespace get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found" 1200
echo "Kafka cluster is ready"

waitAppear "kubectl --context $CTX_HUB get kafkatopic spec -n $targetNamespace --ignore-not-found | grep spec || true"
waitAppear "kubectl --context $CTX_HUB get kafkatopic status -n $targetNamespace --ignore-not-found | grep status || true"
echo "Kafka topics spec and status are ready!"

# kafkaUser=global-hub-kafka-user
# waitAppear "kubectl get secret ${kafkaUser} -n kafka --ignore-not-found"
# echo "Kafka user ${kafkaUser} is ready!"

# generate transport secret
bootstrapServers=$(kubectl --context $CTX_HUB get kafka kafka -n $targetNamespace -o jsonpath='{.status.listeners[1].bootstrapServers}')
kubectl --context $CTX_HUB get kafka kafka -n $targetNamespace -o jsonpath='{.status.listeners[1].certificates[0]}' > $setupDir/config/kafka-ca-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.crt}' | base64 -d > $setupDir/config/kafka-client-cert.pem
# kubectl get secret ${kafkaUser} -n kafka -o jsonpath='{.data.user\.key}' | base64 -d > $setupDir/config/kafka-client-key.pem

# create target namespace
kubectl --context $CTX_HUB create namespace $targetNamespace --dry-run=client -o yaml | kubectl apply -f -
kubectl --context $CTX_HUB create secret generic $transportSecret -n $targetNamespace \
    --from-literal=bootstrap_server=$bootstrapServers \
    --from-file=ca.crt=$setupDir/config/kafka-ca-cert.pem
    # --from-file=client.crt=$setupDir/config/kafka-client-cert.pem \
    # --from-file=client.key=$setupDir/config/kafka-client-key.pem 
echo "transport secret is ready!"


