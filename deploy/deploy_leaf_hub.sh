#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"
script_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

acm_namespace=open-cluster-management

function deploy_custom_repos() {
  # apply custom placement rule operator, appears in the ClusterServiceVersion
  kubectl get ClusterServiceVersion -n "$acm_namespace" -o yaml |
    sed '/kubectl.kubernetes.io\/last-applied-configuration: |/,+1d' |
    grep -v resourceVersion |
    sed 's#registry.redhat.io/rhacm2/multicluster-operators-placementrule-rhel.*$#quay.io/open-cluster-management/multicluster-operators-placementrule:2.4.0-95e830fdea41382aa9d710b5cee83e6c3ae847ab#g' |
    kubectl apply -n "$acm_namespace" -f -

  # apply custom repos that do not appear in the ClusterServiceVersion
  kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
  kubectl create configmap custom-repos --from-file=${script_dir}/leaf_hub_custom_repos.json -n "$acm_namespace"
  kubectl annotate mch multiclusterhub --overwrite mch-imageOverridesCM=custom-repos -n "$acm_namespace"
}

function deploy_hoh_resources() {
  # apply the HoH config CRD
  hoh_config_crd_exists=$(kubectl get crd configs.hub-of-hubs.open-cluster-management.io --ignore-not-found)
  if [[ ! -z "$hoh_config_crd_exists" ]]; then # if exists replace with the requested tag
    kubectl replace -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  else
    kubectl apply -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
  fi

  # create namespace if not exists
  kubectl create namespace hoh-system --dry-run=client -o yaml | kubectl apply -f -
}

function deploy_transport() {
  transport_type=${TRANSPORT_TYPE-kafka}
  base64_command='base64 -w 0'

  if [ "$(uname)" == "Darwin" ]; then
      base64_command='base64'
  fi

  if [ "${transport_type}" == "sync-service" ]; then
    # shellcheck source=deploy/deploy_leaf_hub_sync_servic.sh
    source "${script_dir}/deploy_leaf_hub_sync_service.sh"
    export SYNC_SERVICE_PORT=8090
  else
    bootstrapServers="$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].bootstrapServers})"
    export KAFKA_BOOTSTRAP_SERVERS=$bootstrapServers
    certificate="$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].certificates[0]} | $base64_command)"
    export KAFKA_SSL_CA=$certificate
  fi
}

function deploy_lh_controllers() {
  transport_type=${TRANSPORT_TYPE-kafka}
  if [ "${transport_type}" != "sync-service" ]; then
    transport_type=kafka
  fi

  curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-spec-sync/$TAG/deploy/leaf-hub-spec-sync.yaml.template" |
    IMAGE="quay.io/open-cluster-management-hub-of-hubs/leaf-hub-spec-sync:$TAG" envsubst | kubectl apply -f -
  curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-status-sync/$TAG/deploy/leaf-hub-status-sync.yaml.template" |
    IMAGE="quay.io/open-cluster-management-hub-of-hubs/leaf-hub-status-sync:$TAG" envsubst | kubectl apply -f -
}

deploy_custom_repos
deploy_hoh_resources
deploy_transport
deploy_lh_controllers
