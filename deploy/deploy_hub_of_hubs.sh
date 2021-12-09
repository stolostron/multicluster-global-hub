#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

# install PGO first
rm -rf hub-of-hubs-postgresql
git clone https://github.com/open-cluster-management/hub-of-hubs-postgresql
cd hub-of-hubs-postgresql/pgo
./setup.sh
cd ../../
rm -rf hub-of-hubs-postgresql

pg_namespace="hoh-postgres"
pgo_prefix="hoh-pguser-"
process_user="${pgo_prefix}hoh-process-user"
transport_user="${pgo_prefix}transport-bridge-user"

pg_process_user_URI="$(kubectl get secrets -n "${pg_namespace}" "${process_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
pg_transport_user_URI="$(kubectl get secrets -n "${pg_namespace}" "${transport_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"

# always check whether DATABASE_URL_HOH is set, if not - use pg_process_user_URI
if [[ -z "${DATABASE_URL_HOH-}" ]]; then
  database_url_hoh=$pg_process_user_URI
else
  database_url_hoh=$DATABASE_URL_HOH
fi

# always check whether DATABASE_URL_TRANSPORT is set, if not - use pg_transport_user_URI
if [[ -z "${DATABASE_URL_TRANSPORT-}" ]]; then
  database_url_transport=$pg_transport_user_URI
else
  database_url_transport=$DATABASE_URL_TRANSPORT
fi

acm_namespace=open-cluster-management
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
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$database_url_transport"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/$TAG/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$CSS_SYNC_SERVICE_HOST" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="quay.io/open-cluster-management-hub-of-hubs/hub-of-hubs-spec-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/$TAG/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    SYNC_SERVICE_HOST="$CSS_SYNC_SERVICE_HOST" SYNC_SERVICE_PORT="$css_sync_service_port" IMAGE="quay.io/open-cluster-management-hub-of-hubs/hub-of-hubs-status-transport-bridge:$TAG" envsubst | kubectl apply -f - -n "$acm_namespace"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/data.json" > data.json
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/role_bindings.yaml" > role_bindings.yaml
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/opa_authorization.rego" > opa_authorization.rego

kubectl delete secret opa-data -n "$acm_namespace" --ignore-not-found
kubectl create secret generic opa-data -n "$acm_namespace" --from-file=data.json --from-file=role_bindings.yaml --from-file=opa_authorization.rego

rm -rf data.json role_bindings.yaml opa_authorization.rego

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-rbac envsubst | kubectl apply -f - -n "$acm_namespace"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl apply -f - -n "$acm_namespace"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/ingress.yaml.template" |
    COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl apply -f - -n "$acm_namespace"

# deploy hub-of-hubs-console using its Helm chart. We could have used a helm chart repository, see https://harness.io/blog/helm-chart-repo,
# but here we do it in a simple way, just by cloning the chart repo

rm -rf hub-of-hubs-console-chart
git clone https://github.com/open-cluster-management/hub-of-hubs-console-chart.git
cd hub-of-hubs-console-chart
kubectl annotate mch multiclusterhub mch-pause=true -n "$acm_namespace" --overwrite
kubectl delete appsub console-chart-sub  -n open-cluster-management --ignore-not-found
cat stable/console-chart/values.yaml | sed "s/console: \"\"/console: quay.io/open-cluster-management-hub-of-hubs\/console:$TAG/g" |
    helm upgrade console-chart stable/console-chart -n open-cluster-management --install -f -
cd ..
rm -rf hub-of-hubs-console-chart
