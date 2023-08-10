#!/bin/bash
KUBECONFIG=${1:-$KUBECONFIG}

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: check storage secret
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
storageSecret=${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"}
ready=$(kubectl get secret $storageSecret -n $targetNamespace --ignore-not-found=true)
if [ ! -z "$ready" ]; then
  echo "storageSecret $storageSecret already exists in $TARGET_NAMESPACE namespace"
  exit 0
fi

# step2: deploy postgres operator pgo
kubectl apply --server-side -k ${currentDir}/postgres-operator
waitAppear "kubectl get pods -n postgres-operator --ignore-not-found=true | grep pgo | grep Running || true"
# kubectl -n postgres-operator wait --for=condition=Available Deployment/"pgo" --timeout=1000s

# step3: deploy postgres cluster
kubectl apply -k ${currentDir}/postgres-cluster
waitAppear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"
waitAppear "kubectl get secret hoh-pguser-guest -n hoh-postgres --ignore-not-found=true"

# step4: generate storage secret
pgnamespace="hoh-postgres"
superuserSecret="hoh-pguser-postgres"
readonlyuserSecret="hoh-pguser-guest"
certSecret="hoh-cluster-cert"

superuserDatabaseURI=$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "uri" | base64decode}}')
readonlyuserDatabaseURI=$(kubectl get secrets -n "${pgnamespace}" "${readonlyuserSecret}" -o go-template='{{index (.data) "uri" | base64decode}}')
kubectl get secret $certSecret -n $pgnamespace -o jsonpath='{.data.ca\.crt}' |base64 -d > $currentDir/ca.crt

kubectl create secret generic $storageSecret -n $targetNamespace \
    --from-literal=database_uri="${superuserDatabaseURI}?sslmode=verify-ca" \
    --from-literal=database_uri_with_readonlyuser="${readonlyuserDatabaseURI}?sslmode=verify-ca" \
    --from-file=ca.crt=$currentDir/ca.crt 

echo "storage secret is ready in $targetNamespace namespace!"