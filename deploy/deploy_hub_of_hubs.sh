#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

TAG=${TAG:="v0.1.0"}

PG_NAMESPACE="hoh-postgres"
PGO_PREFIX="hoh-pguser-"
PROCESS_USER="${PGO_PREFIX}hoh-process-user"
TRANSPORT_USER="${PGO_PREFIX}transport-bridge-user"
HOH_USER="${PGO_PREFIX}hoh"

echo "hoh user:" "$(kubectl get secrets -n "${PG_NAMESPACE}" "${HOH_USER}" -o go-template='{{.data.password | base64decode}}')"
#echo "$(kubectl get secrets -n "${PG_NAMESPACE}" "${PROCESS_USER}" -o go-template='{{.data.password | base64decode}}')"
#echo "$(kubectl get secrets -n "${PG_NAMESPACE}" "${TRANSPORT_USER}" -o go-template='{{.data.password | base64decode}}')"

PG_PROCESS_USER_URI="$(kubectl get secrets -n "${PG_NAMESPACE}" "${PROCESS_USER}" -o go-template='{{.data.uri | base64decode}}')"
PG_TRANSPORT_USER_URI="$(kubectl get secrets -n "${PG_NAMESPACE}" "${TRANSPORT_USER}" -o go-template='{{.data.uri | base64decode}}')"

PROCESSOR_URI=${DATABASE_URL_HOH:=$PG_PROCESS_USER_URI}
TRANSPORT_URI=${DATABASE_URL_TRANSPORT:=$PG_TRANSPORT_USER_URI}

echo $PROCESSOR_URI

acm_namespace=open-cluster-management
css_sync_service_host=${CSS_SYNC_SERVICE_HOST:-ec2-107-21-78-35.compute-1.amazonaws.com}
css_sync_service_port=${CSS_SYNC_SERVICE_PORT:-9689}

echo "using kubeconfig $KUBECONFIG"

kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
kubectl create configmap custom-repos --from-file=hub_of_hubs_custom_repos.json -n "$acm_namespace"
kubectl annotate mch multiclusterhub --overwrite mch-imageOverridesCM=custom-repos -n "$acm_namespace"

# apply the HoH config CRD
kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"

# create namespace if not exists
kubectl create namespace hoh-system --dry-run -o yaml | kubectl apply -f -

# apply default HoH config CR
kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/cr-examples/hub-of-hubs.open-cluster-management.io_config_cr.yaml" -n hoh-system

kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-secret -n "$acm_namespace" --from-literal=url="$PROCESSOR_URI"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$TRANSPORT_URI"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/$TAG/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$css_sync_service_host" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-spec-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/$TAG/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$css_sync_service_host" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-status-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
