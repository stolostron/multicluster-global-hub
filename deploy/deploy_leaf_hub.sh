#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

acm_namespace=open-cluster-management
sync_service_port=9689
css_listening_port=8090

echo "using kubeconfig $KUBECONFIG"

kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
kubectl create configmap custom-repos --from-file=leaf_hub_custom_repos.json -n "$acm_namespace"
kubectl annotate mch multiclusterhub  --overwrite mch-imageOverridesCM=custom-repos  -n "$acm_namespace"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-sync-service/$TAG/ess/ess.yaml.template" | \
    CSS_HOST="$SYNC_SERVICE_HOST" CSS_PORT="$sync_service_port" LISTENING_PORT="$css_listening_port" envsubst | kubectl apply -f -
curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-spec-sync/$TAG/deploy/leaf-hub-spec-sync.yaml.template" | \
	SYNC_SERVICE_PORT="$sync_service_port" IMAGE="nirrozenbaumibm/leaf-hub-spec-sync:latest" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-status-sync/$TAG/deploy/leaf-hub-status-sync.yaml.template" | \
    SYNC_SERVICE_PORT="$sync_service_port" IMAGE="nirrozenbaumibm/leaf-hub-status-sync:latest" envsubst | kubectl apply -f - -n "$acm_namespace"
