# !/bin/bash

#!/bin/bash
KUBECONFIG=${1:-$KUBECONFIG}

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"

source $rootDir/test/setup/common.sh

# step1: check storage secret
targetNamespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
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

# step3: deploy  postgres cluster
kubectl --kubeconfig $KUBECONFIG apply -k ${currentDir}/postgres-cluster
waitAppear "kubectl --kubeconfig $KUBECONFIG get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"

# step4: generate storage secret
pgnamespace="hoh-postgres"
userSecret="hoh-pguser-postgres"
certSecret="hoh-cluster-cert"

databaseURI=$(kubectl --kubeconfig $KUBECONFIG get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "uri" | base64decode}}')
kubectl --kubeconfig $KUBECONFIG get secret $certSecret -n $pgnamespace -o jsonpath='{.data.ca\.crt}' |base64 -d > $currentDir/ca.crt

# create target namespace
kubectl --kubeconfig $KUBECONFIG create namespace $targetNamespace || true
kubectl --kubeconfig $KUBECONFIG create secret generic $storageSecret -n $targetNamespace \
    --from-literal=database_uri="${databaseURI}?sslmode=verify-ca" \
    --from-file=ca.crt=$currentDir/ca.crt 

echo "storage secret is ready in $targetNamespace namespace!"

# expose the postgres service as NodePort
pgnamespace="hoh-postgres"
kubectl --kubeconfig $KUBECONFIG patch postgrescluster hoh -n $pgnamespace -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}'  --type merge

stss=$(kubectl --kubeconfig $KUBECONFIG get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl --kubeconfig $KUBECONFIG patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl --kubeconfig $KUBECONFIG delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null  
echo "Postgres is pathed!"
