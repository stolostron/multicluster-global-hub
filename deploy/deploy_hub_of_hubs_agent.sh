#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"
script_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

function deploy_hoh_resources() {
  # apply the HoH config CRD
  hoh_config_crd_exists=$(kubectl get crd configs.hub-of-hubs.open-cluster-management.io --ignore-not-found)
  if [[ ! -z "$hoh_config_crd_exists" ]]; then # if exists replace with the requested tag
    kubectl replace -f "https://raw.githubusercontent.com/stolostron/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  else
    kubectl apply -f "https://raw.githubusercontent.com/stolostron/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  fi

  # create namespace if not exists
  kubectl create namespace hoh-system --dry-run=client -o yaml | kubectl apply -f -
}

function deploy_transport() {
  transport_type=${TRANSPORT_TYPE-kafka}
  # for kafka, no need to deploy anything in the agent
  if [ "${transport_type}" == "sync-service" ]; then
    syncServiceCssHost="$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n sync-service get route sync-service-css -o jsonpath={.status.ingress[0].host})"
    export CSS_SYNC_SERVICE_HOST=$syncServiceCssHost
    export CSS_SYNC_SERVICE_PORT=80 # route of sync service css is exposed on port 80
    # shellcheck source=deploy/deploy_hub_of_hubs_agent_sync_service.sh
    source "${script_dir}/deploy_hub_of_hubs_agent_sync_service.sh"
  fi
}

function deploy_lh_controllers() {
  transport_type=${TRANSPORT_TYPE-kafka}
  if [ "${transport_type}" == "sync-service" ]; then
    export SYNC_SERVICE_PORT=8090
  else
    base64_command='base64 -w 0'

    if [ "$(uname)" == "Darwin" ]; then
      base64_command='base64'
    fi
    transport_type=kafka
    bootstrapServers="$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].bootstrapServers})"
    export KAFKA_BOOTSTRAP_SERVERS=$bootstrapServers
    certificate="$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].certificates[0]} | $base64_command)"
    export KAFKA_SSL_CA=$certificate
  fi

  curl -s "https://raw.githubusercontent.com/stolostron/leaf-hub-spec-sync/$TAG/deploy/leaf-hub-spec-sync.yaml.template" |
    ENFORCE_HOH_RBAC="true" IMAGE="quay.io/open-cluster-management-hub-of-hubs/leaf-hub-spec-sync:$TAG" envsubst | kubectl apply -f -
  curl -s "https://raw.githubusercontent.com/stolostron/leaf-hub-status-sync/$TAG/deploy/leaf-hub-status-sync.yaml.template" |
    IMAGE="quay.io/open-cluster-management-hub-of-hubs/leaf-hub-status-sync:$TAG" envsubst | kubectl apply -f -
}

deploy_hoh_resources
deploy_transport
deploy_lh_controllers
