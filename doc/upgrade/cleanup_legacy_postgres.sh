#!/bin/bash

# Set the namespace, default to "multicluster-global-hub" if not provided
NAMESPACE="${1:-multicluster-global-hub}"

echo ">> Starting cleanup of legacy Postgres resources in namespace: $NAMESPACE"

# Remove the StatefulSet
echo ">> Deleting StatefulSet: multicluster-global-hub-postgres..."
kubectl delete sts multicluster-global-hub-postgres -n $NAMESPACE

# Remove Services
echo ">> Deleting ConfigMaps..."
kubectl delete svc multicluster-global-hub-postgres -n $NAMESPACE

# Remove ConfigMaps
echo ">> Deleting ConfigMaps..."
kubectl delete cm multicluster-global-hub-postgres-ca -n $NAMESPACE
kubectl delete cm multicluster-global-hub-postgres-config -n $NAMESPACE
kubectl delete cm multicluster-global-hub-postgres-init -n $NAMESPACE

# Remove Secrets
echo ">> Deleting Secrets..."
kubectl delete secret postgres-credential-secret -n $NAMESPACE
kubectl delete secret multicluster-global-hub-postgres -n $NAMESPACE
kubectl delete secret multicluster-global-hub-postgres-certs -n $NAMESPACE

echo ">> Cleanup completed for namespace: $NAMESPACE"
