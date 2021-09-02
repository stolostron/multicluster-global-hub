#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

acm_namespace=open-cluster-management
ess_sync_service_listening_port=8090

echo "using kubeconfig $KUBECONFIG"

kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
kubectl create configmap custom-repos --from-file=leaf_hub_custom_repos.json -n "$acm_namespace"
kubectl annotate mch multiclusterhub  --overwrite mch-imageOverridesCM=custom-repos  -n "$acm_namespace"

# apply the HoH config CRD
kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"

curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-spec-sync/$TAG/deploy/leaf-hub-spec-sync.yaml.template" | \
	SYNC_SERVICE_PORT="$ess_sync_service_listening_port" IMAGE="nirrozenbaumibm/leaf-hub-spec-sync:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-status-sync/$TAG/deploy/leaf-hub-status-sync.yaml.template" | \
    SYNC_SERVICE_PORT="$ess_sync_service_listening_port" IMAGE="nirrozenbaumibm/leaf-hub-status-sync:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
