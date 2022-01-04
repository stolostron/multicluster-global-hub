#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"
script_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
acm_namespace=open-cluster-management

function deploy_hoh_resources() {
  # apply the HoH config CRD
  hoh_config_crd_exists=$(kubectl get crd configs.hub-of-hubs.open-cluster-management.io --ignore-not-found)
  if [[ ! -z "$hoh_config_crd_exists" ]]; then # if exists replace with the requested tag
    kubectl replace -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/v0.2.0/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  else
    kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/v0.2.0/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  fi

  # create namespace if not exists
  kubectl create namespace hoh-system --dry-run=client -o yaml | kubectl apply -f -

  # apply default HoH config CR
  kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/v0.2.0/cr-examples/hub-of-hubs.open-cluster-management.io_config_cr.yaml" -n hoh-system
}

function deploy_transport() {
  ## if TRANSPORT_TYPE is sync service, set sync service env vars, otherwise any other value will result in kafka being selected as transport
  transport_type=${TRANSPORT_TYPE-kafka}
  if [ "${transport_type}" == "sync-service" ]; then
    # TODO deploy sync service in cluster
    export SYNC_SERVICE_HOST="$CSS_SYNC_SERVICE_HOST"
    export SYNC_SERVICE_PORT=${CSS_SYNC_SERVICE_PORT:-9689}
  else
    # shellcheck source=deploy/deploy_kafka.sh
    source "${script_dir}/deploy_kafka.sh"
  fi
}

function deploy_hoh_controllers() {
  database_url_hoh=$1
  database_url_transport=$2

  kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found
  kubectl create secret generic hub-of-hubs-database-secret -n "$acm_namespace" --from-literal=url="$database_url_hoh"
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl apply -f - -n "$acm_namespace"
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-status-sync envsubst | kubectl apply -f - -n "$acm_namespace"

  kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --ignore-not-found
  kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$acm_namespace" --from-literal=url="$database_url_transport"

  transport_type=${TRANSPORT_TYPE-kafka}
  if [ "${transport_type}" != "sync-service" ]; then
    transport_type=kafka
  fi

  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/v0.2.0/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    TRANSPORT_TYPE="${transport_type}" IMAGE="quay.io/open-cluster-management-hub-of-hubs/hub-of-hubs-spec-transport-bridge:v0.2.0" envsubst | kubectl apply -f - -n "$acm_namespace"
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/v0.2.0/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    TRANSPORT_TYPE="${transport_type}" IMAGE="quay.io/open-cluster-management-hub-of-hubs/hub-of-hubs-status-transport-bridge:v0.2.0" envsubst | kubectl apply -f - -n "$acm_namespace"
}

function deploy_rbac() {
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/data.json" > ${script_dir}/data.json
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/role_bindings.yaml" > ${script_dir}/role_bindings.yaml
  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/opa_authorization.rego" > ${script_dir}/opa_authorization.rego

  kubectl delete secret opa-data -n "$acm_namespace" --ignore-not-found
  kubectl create secret generic opa-data -n "$acm_namespace" --from-file=${script_dir}/data.json --from-file=${script_dir}/role_bindings.yaml --from-file=${script_dir}/opa_authorization.rego

  rm -rf ${script_dir}/data.json ${script_dir}/role_bindings.yaml ${script_dir}/opa_authorization.rego

  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-rbac envsubst | kubectl apply -f - -n "$acm_namespace"

  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl apply -f - -n "$acm_namespace"

  curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/ingress.yaml.template" |
    COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl apply -f - -n "$acm_namespace"
}

function deploy_helm_charts() {
  # deploy hub-of-hubs-console using its Helm chart.
  #
  # We could have used a helm chart repository,
  # see https://harness.io/blog/helm-chart-repo,
  # but here we do it in a simple way, just by cloning the chart repos
  kubectl annotate mch multiclusterhub mch-pause=true -n "$acm_namespace" --overwrite

  rm -rf hub-of-hubs-console-chart
  git clone https://github.com/open-cluster-management/hub-of-hubs-console-chart.git
  cd hub-of-hubs-console-chart
  git checkout $TAG
  helm get values -a -n "$acm_namespace" $(helm ls -n "$acm_namespace" | cut -d' ' -f1 | grep console-chart) -o yaml > values.yaml
  kubectl delete appsub console-chart-sub -n "$acm_namespace" --ignore-not-found
  cat values.yaml |
      yq e ".global.imageOverrides.console = \"quay.io/open-cluster-management-hub-of-hubs/console:$TAG\"" - |
      yq e '.global.pullPolicy = "Always"' - |
      helm upgrade console-chart stable/console-chart -n "$acm_namespace" --install -f -
  cd ..
  rm -rf hub-of-hubs-console-chart

  rm -rf grc-chart
  git clone https://github.com/open-cluster-management/grc-chart.git
  cd grc-chart
  git fetch origin release-2.4
  git checkout release-2.4
  helm get values -a -n "$acm_namespace" $(helm ls -n "$acm_namespace" | cut -d' ' -f1 | grep grc) -o yaml > values.yaml
  kubectl delete appsub grc-sub -n "$acm_namespace" --ignore-not-found

  cat values.yaml |
      yq e ".global.imageOverrides.governance_policy_propagator = \"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:no_status_update\"" - |
      yq e ".global.imageOverrides.grc_ui = \"quay.io/open-cluster-management-hub-of-hubs/grc-ui:$TAG\"" - |
      yq e '.global.pullPolicy = "Always"' - |
      helm upgrade grc stable/grc -n "$acm_namespace" --install -f -
  cd ..
  rm -rf grc-chart
}

# always check whether DATABASE_URL_HOH and DATABASE_URL_TRANSPORT are set, if not - install PGO and use its secrets
if [ -z "${DATABASE_URL_HOH-}" ] && [ -z "${DATABASE_URL_TRANSPORT-}" ]; then
  rm -rf hub-of-hubs-postgresql
  git clone https://github.com/open-cluster-management/hub-of-hubs-postgresql
  cd hub-of-hubs-postgresql/pgo
  git checkout $TAG
  IMAGE=quay.io/open-cluster-management-hub-of-hubs/postgresql-ansible:$TAG ./setup.sh
  cd ../../
  rm -rf hub-of-hubs-postgresql

  pg_namespace="hoh-postgres"
  process_user="hoh-pguser-hoh-process-user"
  transport_user="hoh-pguser-transport-bridge-user"

  database_url_hoh="$(kubectl get secrets -n "${pg_namespace}" "${process_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
  database_url_transport="$(kubectl get secrets -n "${pg_namespace}" "${transport_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
else
  database_url_hoh=$DATABASE_URL_HOH
  database_url_transport=$DATABASE_URL_TRANSPORT
fi

deploy_hoh_resources
deploy_transport
deploy_hoh_controllers "$database_url_hoh" "$database_url_transport"
deploy_rbac
deploy_helm_charts
