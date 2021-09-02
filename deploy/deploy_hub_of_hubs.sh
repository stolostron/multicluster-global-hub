#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

acm_namespace=open-cluster-management
css_sync_service_port=9689

echo "using kubeconfig $KUBECONFIG"

kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
kubectl create configmap custom-repos --from-file=hub_of_hubs_custom_repos.json -n "$acm_namespace"
kubectl annotate mch multiclusterhub  --overwrite mch-imageOverridesCM=custom-repos  -n "$acm_namespace"

# apply the HoH config CRD
kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"

# create namespace if not exists
kubectl create namespace hoh-system --dry-run -o yaml | kubectl apply -f -

# apply default HoH config CR
kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/cr-examples/hub-of-hubs.open-cluster-management.io_config_cr.yaml" -n hoh-system

kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-secret -n "$acm_namespace" --from-literal=url="$DATABASE_URL_HOH"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" | \
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" | \
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$DATABASE_URL_TRANSPORT"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/$TAG/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" | \
    SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-spec-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/$TAG/deploy/hub-of-hubs-status-transport-bridge.yaml.template" | \
    SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-status-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
