#!/bin/bash

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." ; pwd -P)"
source $rootDir/test/setup/common.sh

# step1: delete transport secret
targetNamespace=${TARGET_NAMESPACE:-"open-cluster-management"}
storageSecret=${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"}
kubectl delete secret $storageSecret -n $targetNamespace
echo "deletes storage secret $storageSecret from namespace $targetNamespace"

# step2: delete postgres cluster
kubectl delete -k ${currentDir}/postgres-cluster
waitDisappear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"
echo "postgres cluster is deleted"

# step3: delete postgres operator
kubectl delete -k ${currentDir}/postgres-operator
waitDisappear "kubectl get deploy pgo -n postgres-operator --ignore-not-found=true"
echo "postgres operator: pgo is deleted"