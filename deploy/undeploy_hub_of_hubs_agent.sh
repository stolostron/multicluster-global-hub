#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset
branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi

acm_namespace=open-cluster-management

echo "using kubeconfig $KUBECONFIG"
kubectl delete namespace hoh-system --ignore-not-found

curl -s "https://raw.githubusercontent.com/stolostron/leaf-hub-spec-sync/$branch/deploy/leaf-hub-spec-sync.yaml.template" | \
    envsubst | kubectl delete -f - --ignore-not-found
curl -s "https://raw.githubusercontent.com/stolostron/leaf-hub-status-sync/$branch/deploy/leaf-hub-status-sync.yaml.template" | \
    envsubst | kubectl delete -f - --ignore-not-found
    
# delete the HoH config CRD
kubectl delete -f "https://raw.githubusercontent.com/stolostron/hub-of-hubs-crds/$branch/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml" \
	--ignore-not-found

# delete sync-service namespace if exists - this will also delete all living resources inside the sync-service namespace
kubectl delete namespace sync-service --ignore-not-found

kubectl annotate mch multiclusterhub  --overwrite mch-imageOverridesCM=  -n "$acm_namespace"
kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
