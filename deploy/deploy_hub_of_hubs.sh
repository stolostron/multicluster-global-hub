#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

TAG=${TAG:="v0.1.0"}

pg_namespace="hoh-postgres"
pgo_prefix="hoh-pguser-"
process_user="${pgo_prefix}hoh-process-user"
transport_user="${pgo_prefix}transport-bridge-user"

pg_process_user_URI="$(kubectl get secrets -n "${pg_namespace}" "${process_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
pg_transport_user_URI="$(kubectl get secrets -n "${pg_namespace}" "${transport_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"

database_url_hoh=${DATABASE_URL_HOH:=$pg_process_user_URI}
database_url_transport=${DATABASE_URL_TRANSPORT:=$pg_transport_user_URI}

acm_namespace=open-cluster-management
css_sync_service_host=${CSS_SYNC_SERVICE_HOST:-my-postgres.com}
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
kubectl create secret generic hub-of-hubs-database-secret -n "$acm_namespace" --from-literal=url="$database_url_hoh"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$database_url_transport"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/$TAG/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$css_sync_service_host" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-spec-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/$TAG/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$css_sync_service_host" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="nirrozenbaumibm/hub-of-hubs-status-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/testdata/data.json" > data.json
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/role_bindings.yaml" > role_bindings.yaml
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/opa_authorization.rego" > opa_authorization.rego

kubectl delete secret opa-data -n "$acm_namespace" --ignore-not-found
kubectl create secret generic opa-data -n "$acm_namespace" --from-file=data.json --from-file=role_bindings.yaml --from-file=opa_authorization.rego

rm -rf data.json role_bindings.yaml opa_authorization.rego

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/deploy/operator.yaml.template" |
    REGISTRY=vadimeisenbergibm IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-rbac envsubst | kubectl apply -f - -n "$acm_namespace"
