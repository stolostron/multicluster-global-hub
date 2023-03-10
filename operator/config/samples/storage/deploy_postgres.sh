#!/bin/bash
KUBECONFIG=${1:-$KUBECONFIG}

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: check storage secret
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
storageSecret=${STORAGE_SECRET_NAME:-"storage-secret"}
ready=$(kubectl get secret $storageSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "storageSecret $storageSecret already exists in $TARGET_NAMESPACE namespace"
  exit 0
fi

# step2: deploy postgres operator pgo
kubectl apply --server-side -k ${currentDir}/postgres-operator
waitAppear "kubectl get pods -n postgres-operator --ignore-not-found=true | grep pgo | grep Running || true"
# kubectl -n postgres-operator wait --for=condition=Available Deployment/"pgo" --timeout=1000s

# step3: deploy  postgres cluster
kubectl apply -k ${currentDir}/postgres-cluster
waitAppear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"

# step4: generate storage secret
pgnamespace="hoh-postgres"
userSecret="hoh-pguser-postgres"
certSecret="hoh-cluster-cert"
databaseHost="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "host" | base64decode}}')"
databasePort="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "port" | base64decode}}')"
databaseUser="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "user" | base64decode}}')"
databasePassword="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "password" | base64decode}}')"
databasePwd=$(printf %s "$databasePassword" |jq -sRr @uri)

kubectl get secret $certSecret -n $pgnamespace -o jsonpath='{.data.ca\.crt}' |base64 -d > $currentDir/ca.crt

kubectl create secret generic $storageSecret -n $targetNamespace \
    --from-literal=database_uri="postgres://${databaseUser}:${databasePwd}@${databaseHost}:${databasePort}/hoh" \
    --from-file=ca.crt=$currentDir/ca.crt \
    --from-file=tls.crt=$currentDir/tls.crt \
    --from-file=tls.key=$currentDir/tls.key
echo "storage secret is ready in $targetNamespace namespace!"