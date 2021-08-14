#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

acm_namespace=open-cluster-management
sync_service_port=9689

echo "using kubeconfig $KUBECONFIG"

# apply the HoH config CRD
kubectl apply -f https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/main/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml

kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-secret -n "$acm_namespace" --from-literal=url="$DATABASE_URL_HOH"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" | IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" | IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$DATABASE_URL_TRANSPORT"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/main/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" | SYNC_SERVICE_PORT="$sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-spec-transport-bridge:rbac" envsubst | kubectl apply -f - -n "$acm_namespace"
