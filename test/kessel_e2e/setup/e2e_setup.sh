#!/bin/bash

#!/usr/bin/env bash

set -exo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}

function initKinDCluster() {
  clusterName="$1"
  if [[ $(kind get clusters | grep "^${clusterName}$" || true) != "${clusterName}" ]]; then
    kind create cluster --name "$clusterName" --wait 1m
    currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    kubectl config view --context="kind-${clusterName}" --minify --flatten > ${currentDir}/kubeconfig-${clusterName}
  fi
}

enableRouter() {
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl --context "$1" apply -f -
  GIT_PATH="https://raw.githubusercontent.com/openshift/router/release-4.16"
  kubectl --context "$1" apply -f $GIT_PATH/deploy/route_crd.yaml
  # pacman application depends on route crd, but we do not need to have route pod running in the cluster
  # kubectl apply -f $GIT_PATH/deploy/router.yaml
  # kubectl apply -f $GIT_PATH/deploy/router_rbac.yaml
}

enableServiceCA() {
  HUB_OF_HUB_NAME=$2
  # apply service-ca
  kubectl --context $1 label node ${HUB_OF_HUB_NAME}-control-plane node-role.kubernetes.io/master=
  kubectl --context $1 apply -f ${CURRENT_DIR}/service-ca-crds
  kubectl --context $1 create ns openshift-config-managed
  kubectl --context $1 apply -f ${CURRENT_DIR}/service-ca/
}

# deploy olm
function enableOLM() {
  NS=olm
  csvPhase=$(kubectl --context "$1" get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  if [[ "$csvPhase" == "Succeeded" ]]; then
    echo "OLM is already installed in ${NS} namespace. Exiting..."
    exit 1
  fi
  
  GIT_PATH="https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/v0.28.0"
  kubectl --context "$1" apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  kubectl --context "$1" wait --for=condition=Established -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml" --timeout=60s
  kubectl --context "$1" apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"

  # apply proxies.config.openshift.io which is required by olm
  kubectl --context "$1" apply -f "https://raw.githubusercontent.com/openshift/api/master/payload-manifests/crds/0000_03_config-operator_01_proxies.crd.yaml"

  retries=60
  csvPhase=$(kubectl --context "$1" get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  while [[ $retries -gt 0 && "$csvPhase" != "Succeeded" ]]; do
    echo "csvPhase: ${csvPhase}"
    sleep 1
    retries=$((retries - 1))
    csvPhase=$(kubectl --context "$1" get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  done
  kubectl --context "$1" rollout status -w deployment/packageserver --namespace="${NS}" --timeout=60s

  if [ $retries == 0 ]; then
    echo "CSV \"packageserver\" failed to reach phase succeeded"
    exit 1
  fi
  echo "CSV \"packageserver\" install succeeded"
}

# deploy global hub
function deployGlobalHub() {
  # build images
  cd ${CURRENT_DIR}/../../../
  MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-operator:latest"
  MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-manager:latest"
  MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-agent:latest"
  docker build . -t $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF -f operator/Dockerfile
  docker build . -t $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF -f manager/Dockerfile
  docker build . -t $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF -f agent/Dockerfile

  # load to kind cluster
  kind load docker-image $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF --name $2
  kind load docker-image $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF --name $2
  #kind load docker-image $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF --name $2

  # replace to use the built images
  sed -i -e "s;quay.io/stolostron/multicluster-global-hub-manager:latest;$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF;" ./operator/config/manager/manager.yaml
  sed -i -e "s;quay.io/stolostron/multicluster-global-hub-agent:latest;$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF;" ./operator/config/manager/manager.yaml
  # update imagepullpolicy to IfNotPresent
  sed -i -e "s;imagePullPolicy: Always;imagePullPolicy: IfNotPresent;" ./operator/config/manager/manager.yaml

  # deploy serviceMonitor CRD
  kubectl --context "$1" apply -f ${CURRENT_DIR}/../../manifest/crd/0000_04_monitoring.coreos.com_servicemonitors.crd.yaml
  # deploy global hub operator
  # Get KinD cluster IP Address
  global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' global-hub-control-plane)
  # patch imagePullSecret as IfNotPresent
  cd operator; make deploy IMG=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF
  cat <<EOF | kubectl --context "$1" apply -f -
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    global-hub.open-cluster-management.io/catalog-source-name: operatorhubio-catalog
    global-hub.open-cluster-management.io/catalog-source-namespace: olm
    global-hub.open-cluster-management.io/with-inventory: ""
    global-hub.open-cluster-management.io/enable-kraft: ""
    global-hub.open-cluster-management.io/kafka-broker-advertised-host: "$global_hub_node_ip"
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
spec:
  availabilityConfig: High
  dataLayer:
    kafka:
      topics:
        specTopic: gh-spec
        statusTopic: gh-event.*
      storageSize: 1Gi
    postgres:
      retention: 18m
      storageSize: 1Gi
  enableMetrics: false
  imagePullPolicy: IfNotPresent
EOF
}

wait_cmd() {
  local command=$1
  local seconds=${2:-"600"}
  local interval=60        # Interval for updating the waiting message
  local command_interval=20 # Interval for executing the command
  local signs=(🙉 🙈 🙊)
  local elapsed=0
  local last_command_run=0

  echo -e "\r${CYAN}$1 $NC "
  if eval "${command}"; then
    return 0 
  fi

  while [ $elapsed -le "$seconds" ]; do
    if [ $((elapsed - last_command_run)) -ge $command_interval ]; then
      if [ -n "$(eval ${command})" ]; then
        return 0 # Return success status code
      fi
      last_command_run=$elapsed
    fi

    if [ $elapsed -eq 0 ]; then
      echo -e "\r placeholder will be overwrite by wating message"
    fi
    local index=$((elapsed / interval % ${#signs[@]}))
    echo -ne "\r ${signs[$index]} Waiting $elapsed seconds: $1"
    sleep $interval
    ((elapsed += interval))
    kubectl get pod -A
    kubectl get kafka -n multicluster-global-hub -oyaml || true
    kubectl get mcgh -n multicluster-global-hub -oyaml || true
    kubectl logs deploy/multicluster-global-hub-operator -n multicluster-global-hub
    kubectl get deploy -n multicluster-global-hub
  done

  echo -e "\n$RED Timeout $seconds seconds $NC: $command"
  return 1 # Return failure status code
}

function wait_global_hub_ready() {
  wait_cmd "kubectl get deploy/multicluster-global-hub-manager -n multicluster-global-hub --context $1"
  kubectl wait deploy/multicluster-global-hub-manager -n multicluster-global-hub --for condition=Available=True --timeout=600s --context "$1"
  wait_cmd "kubectl get deploy/inventory-api -n multicluster-global-hub --context $1"
  wait_cmd "kubectl wait deploy/inventory-api -n multicluster-global-hub --for condition=Available=True --timeout=60s --context "$1""
  kubectl get pod -n multicluster-global-hub --context $1
}

initKinDCluster global-hub
enableRouter kind-global-hub
enableServiceCA kind-global-hub global-hub 
enableOLM kind-global-hub
deployGlobalHub kind-global-hub global-hub
wait_global_hub_ready kind-global-hub