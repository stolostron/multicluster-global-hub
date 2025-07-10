#!/bin/bash

# Version
export INSTALL_DIR=/usr/local/bin
export PATH=$INSTALL_DIR:$PATH
export GRC_VERSION=v0.15.0
export KUBECTL_VERSION=v1.28.1
export CLUSTERADM_VERSION=0.10.1
export KIND_VERSION=v0.19.0
export ROUTE_VERSION=release-4.12
export GO_VERSION=go1.23.6
export GINKGO_VERSION=v2.17.2

# Environment Variables
CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
TEST_DIR=$(dirname "$CURRENT_DIR")
PROJECT_DIR=$(dirname "$TEST_DIR")
export CURRENT_DIR
export TEST_DIR
export PROJECT_DIR
export GH_NAME="global-hub"
export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}
export KinD=true
export CONFIG_DIR=$CURRENT_DIR/config
export GH_KUBECONFIG=$CONFIG_DIR/$GH_NAME

check_dir() {
  if [ ! -d "$1" ]; then
    mkdir "$1"
  fi
}

hover() {
  local pid=$1
  message=${2:-Processing!}
  delay=0.4
  current_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  echo "$pid" >"${current_dir}/config/pid"
  signs=(🙉 🙈 🙊)
  while (kill -0 "$pid" 2>/dev/null); do
    if [[ $LOG =~ "/dev/"* ]]; then
      sleep $delay
    else
      index="${RANDOM} % ${#signs[@]}"
      printf "\e[38;5;$((RANDOM % 257))m%s\r\e[0m" "[$(date '+%H:%M:%S')]  ${signs[${index}]}  ${message} ..."
      sleep ${delay}
    fi
  done
  printf "%s\n" "[$(date '+%H:%M:%S')]  ✅  ${message}"
}

check_kubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
      curl -LO https://storage.googleapis.com/kubernetes-release/release/$KUBECTL_VERSION/bin/darwin/amd64/kubectl
    fi
    chmod +x ./kubectl
    sudo mv ./kubectl ${INSTALL_DIR}/kubectl
  fi
  echo "kubectl version: $(kubectl version --client)"
}

check_helm() {
  if ! command -v helm >/dev/null 2>&1; then
    HELM_INSTALL_DIR=$INSTALL_DIR
    curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  fi
  echo "kubectl version: $(kubectl version --client)"
}

check_kustomize() {
  if ! command -v kustomize >/dev/null 2>&1; then
    GOBIN=$INSTALL_DIR && go install sigs.k8s.io/kustomize/kustomize/v5@v5.4.2
  fi
  echo "kustomize version: $(kustomize version)"
}

check_clusteradm() {
  if ! command -v clusteradm >/dev/null 2>&1; then
    # curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
    curl -LO https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/v$CLUSTERADM_VERSION/install.sh
    chmod +x ./install.sh
    export INSTALL_DIR=$INSTALL_DIR
    source ./install.sh $CLUSTERADM_VERSION
    rm ./install.sh
  fi
  echo "clusteradm path: $(which clusteradm)"
}

check_kind() {
  if ! command -v kind >/dev/null 2>&1; then
    echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
    curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-$(uname)-amd64"
    chmod +x ./kind-amd64
    sudo mv ./kind-amd64 $INSTALL_DIR/kind
  fi
  if [[ $(kind version) < "kind v0.19.0" ]]; then
    echo "KinD version is less than 0.19, update to $KIND_VERSION"
    curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-$(uname)-amd64"
    chmod +x ./kind-amd64
    sudo mv ./kind-amd64 $INSTALL_DIR/kind
  fi
  echo "kind version: $(kind version)"
}

kind_cluster() {
  dir="${CONFIG_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  local cluster_name="$1"
  local kubeconfig="$dir/$cluster_name"
  local max_retries=5
  local counter=0
  local kind_image="${2:-}"

  while [ $counter -lt $max_retries ] && ! (kind get clusters 2>/dev/null | grep -xq "$cluster_name"); do
    echo "creating cluster $cluster_name"
    ensure_cluster "$cluster_name" "$kubeconfig" "$kind_image"
    counter=$((counter + 1))
  done
  if [ $counter -eq $max_retries ]; then
    echo "Failed to create cluster $cluster_name"
    exit 1
  fi
  echo "kind clusters: $(kind get clusters)"
}

ensure_cluster() {
  local cluster_name="$1"
  local kubeconfig="$2"
  local kind_image="$3"

  if kind get clusters 2>/dev/null | grep -xq "$cluster_name"; then
    kind delete cluster --name="$cluster_name"
  fi

  if [ -n "$kind_image" ]; then
    kind create cluster --name "$cluster_name" --image "$kind_image" --wait 5m
  else
    kind create cluster --name "$cluster_name" --wait 5m
  fi

  # modify the context = KinD cluster name = kubeconfig name
  kubectl config delete-context "$cluster_name" 2>/dev/null || true
  kubectl config rename-context "kind-$cluster_name" "$cluster_name"

  # modify the apiserver, so that the spoken cluster can use the kubeconfig to connect it:  governance-policy-framework-addon
  local node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$1-control-plane")

  kubectl config set-cluster "kind-$cluster_name" --server="https://$node_ip:6443"
  kubectl config view --context="$cluster_name" --minify --flatten >"$kubeconfig"
}

init_hub() {
  echo -e "${CYAN} Init Hub $1 ... $NC"
  clusteradm init --wait --context "$1" >/dev/null 2>&1 # not echo the senetive information
  kubectl wait deployment -n open-cluster-management cluster-manager --for condition=Available=True --timeout=200s --context "$1"
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-controller --for condition=Available=True --timeout=200s --context "$1"
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-webhook --for condition=Available=True --timeout=200s --context "$1"
  dir="${CONFIG_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  join_file="$dir/join-$1"
  clusteradm get token --context "$1" | grep "clusteradm" >"$join_file"
}

init_managed() {
  local hub="$1"
  local managed_prefix_name="$2"
  local managed_cluster_num="$3"
  local managed_cluster_name=""

  dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  join_cmd="${dir}/config/join-${hub}"
  for i in $(seq 1 "${managed_cluster_num}"); do
    local managed="${managed_prefix_name}$i"
    if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "$managed" || true) == "" ]]; then
      kubectl config use-context "$managed"
      clusteradm get token --context "${hub}" | grep "clusteradm" >"$join_cmd"
      if [[ $hub =~ "kind" ]]; then
        sed -e "s;<cluster_name>;$managed --force-internal-endpoint-lookup --wait;" "$join_cmd" >"${join_cmd}-named"
        sed -e "s;<cluster_name>;$managed --force-internal-endpoint-lookup --wait;" "$join_cmd" | bash
      else
        sed -e "s;<cluster_name>;$managed --wait;" "$join_cmd" >"${join_cmd}-named"
        sed -e "s;<cluster_name>;$managed --wait;" "$join_cmd" | bash
      fi
    fi
    managed_cluster_name+="$managed,"
  done
  timeout 5m clusteradm accept --clusters "${managed_cluster_name%,*}" --context "${hub}" --wait
}

join_cluster() {
  local hub=$1 # hub name also as the context
  local cluster=$2
  echo -e "${CYAN} Import Cluster $2 to Hub $1 ... $NC"
  dir="${CONFIG_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  join_file="$dir/join-$1"
  if [[ -z $(kubectl get mcl "$cluster" --context "$hub" --ignore-not-found) ]]; then
    # shellcheck disable=SC2154
    if [ "$KinD" = true ]; then
      sed -e "s;<cluster_name>;$cluster --force-internal-endpoint-lookup --context $cluster --wait;" "$join_file" | bash
    else
      sed -e "s;<cluster_name>;$cluster --context $cluster --wait;" "$join_file" | bash
    fi
    timeout 1m clusteradm accept --clusters "$2" --context "${hub}" --wait
  fi
}

# init application-lifecycle
init_app() {
  echo -e "${CYAN} Init Application $1:$2 $NC"
  local hub=$1
  local cluster=$2

  # Use other branch throw error: unable to read URL "https://raw.githubusercontent.com/kubernetes-sigs/application/v0.8.0/deploy/kube-app-manager-aio.yaml", server reported 404 Not Found, status code=404
  app_path=https://raw.githubusercontent.com/kubernetes-sigs/application/master
  kubectl apply -f $app_path/deploy/kube-app-manager-aio.yaml --context "$hub"
  kubectl apply -f $app_path/deploy/kube-app-manager-aio.yaml --context "$cluster"

  # deploy the subscription operators to the hub cluster
  if ! kubectl get deploy/multicluster-operators-subscription -n open-cluster-management --context "$hub"; then
    retry "clusteradm install hub-addon --names application-manager --context $hub && (kubectl get deploy/multicluster-operators-subscription -n open-cluster-management --context $hub)"
  fi

  # enable the addon on the managed clusters
  retry "clusteradm addon enable --names application-manager --clusters $cluster --context $hub" 10
}

init_policy() {
  echo -e "${CYAN} Init Policy $1:$2 $NC"
  local hub=$1
  local cluster=$2

  # create namespace fist
  HUB_NAMESPACE="open-cluster-management"
  kubectl create ns "${HUB_NAMESPACE}" --dry-run=client -o yaml | kubectl --context $hub apply -f -
  MANAGED_NAMESPACE="open-cluster-management-agent-addon"
  kubectl create ns "${MANAGED_NAMESPACE}" --dry-run=client -o yaml | kubectl --context "$cluster" apply -f -

  # Reference: https://open-cluster-management.io/getting-started/integration/policy-framework/
  PROPAGATOR_GIT_HTTP_PATH="https://github.com/open-cluster-management-io/governance-policy-propagator.git"
  propagator="governance-policy-propagator"
  if [ ! -d $propagator ]; then
    git clone $PROPAGATOR_GIT_HTTP_PATH
    cd $propagator
    git checkout $GRC_VERSION
    cd ../
  fi

  # On hub
  if ! kubectl --context $hub get deploy -n "$HUB_NAMESPACE" | grep -q $propagator | grep -q Running; then
    ## Apply the CRDs
    kubectl --context $hub apply -f $propagator/deploy/crds/policy.open-cluster-management.io_policies.yaml
    kubectl --context $hub apply -f $propagator/deploy/crds/policy.open-cluster-management.io_placementbindings.yaml
    kubectl --context $hub apply -f $propagator/deploy/crds/policy.open-cluster-management.io_policyautomations.yaml
    kubectl --context $hub apply -f $propagator/deploy/crds/policy.open-cluster-management.io_policysets.yaml
    # Deploy the policy-propagator
    kubectl --context $hub apply -f $propagator/deploy/operator.yaml -n ${HUB_NAMESPACE}
    sleep 2

    # Replace the laster image with the grc version
    kubectl --context $hub set image deployment/governance-policy-propagator governance-policy-propagator=quay.io/open-cluster-management/governance-policy-propagator:$GRC_VERSION -n ${HUB_NAMESPACE}

    ## Disable the webhook for the controller
    kubectl --context $hub patch deployment $propagator --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--enable-webhooks=false"}]' -n ${HUB_NAMESPACE}
    kubectl --context $hub patch deployment $propagator --type='json' -p='[
    {"op": "remove", "path": "/spec/template/spec/containers/0/volumeMounts/0"},
    {"op": "remove", "path": "/spec/template/spec/volumes/0"} 
    ]' -n ${HUB_NAMESPACE}
  fi

  # On cluster
  ## Create the secret to authenticate with the hub
  dir="${CONFIG_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  local hub_kubeconfig="$dir/$hub"
  if ! kubectl get secret hub-kubeconfig -n "${MANAGED_NAMESPACE}" --context "$cluster"; then
    kubectl --context "$cluster" -n "${MANAGED_NAMESPACE}" create secret generic hub-kubeconfig --from-file=kubeconfig="$hub_kubeconfig"
  fi

  ## Apply the policy CRD
  kubectl --context "$cluster" apply -f $propagator/deploy/crds/policy.open-cluster-management.io_policies.yaml

  ## Deploy the synchronization component

  DEPLOY_ON_HUB=false                   # Set whether or not this is being deployed on the Hub
  local MANAGED_CLUSTER_NAME="$cluster" # Set the managed cluster name and create the namespace
  kubectl create ns "$MANAGED_CLUSTER_NAME" --dry-run=client -o yaml | kubectl --context "$cluster" apply -f -

  POLICY_ADDON_GIT_HTTP_PATH="https://github.com/open-cluster-management-io/governance-policy-framework-addon.git"
  policy_addon=governance-policy-framework-addon
  if [ ! -d $policy_addon ]; then
    git clone $POLICY_ADDON_GIT_HTTP_PATH
    cd $policy_addon
    git checkout $GRC_VERSION
    cd ../
  fi
  pwd
  ls -l
  retry "(kubectl --context $cluster apply -f $policy_addon/deploy/operator.yaml -n $MANAGED_NAMESPACE) && (kubectl --context $cluster get deploy/$policy_addon -n $MANAGED_NAMESPACE)" 10

  kubectl --context "$cluster" set image deployment/$policy_addon $policy_addon=quay.io/open-cluster-management/governance-policy-framework-addon:$GRC_VERSION -n ${MANAGED_NAMESPACE}
  kubectl --context "$cluster" patch deployment $policy_addon -n ${MANAGED_NAMESPACE} \
    -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-framework-addon\",\"args\":[\"--hub-cluster-configfile=/var/run/klusterlet/kubeconfig\", \"--cluster-namespace=${MANAGED_CLUSTER_NAME}\", \"--enable-lease=true\", \"--log-level=2\", \"--disable-spec-sync=${DEPLOY_ON_HUB}\"]}]}}}}"

  ## Install configuration policy controller:
  ## https://open-cluster-management.io/getting-started/integration/policy-controllers/configuration-policy/
  config_policy="config-policy-controller"
  CONFIG_POLICY_GIT_HTTP_PATH="https://github.com/open-cluster-management-io/config-policy-controller.git"
  if [ ! -d $config_policy ]; then
    git clone $CONFIG_POLICY_GIT_HTTP_PATH
    cd $config_policy
    git checkout $GRC_VERSION
    cd ../
  fi
  retry "(kubectl --context $cluster apply -f ${config_policy}/deploy/crds/policy.open-cluster-management.io_configurationpolicies.yaml"
  # kubectl --context "$cluster" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_operatorpolicies.yaml
  retry "(kubectl --context $cluster apply -f ${config_policy}/deploy/operator.yaml -n $MANAGED_NAMESPACE) && (kubectl --context $cluster get deploy/$config_policy -n $MANAGED_NAMESPACE)"

  kubectl --context "$cluster" set image deployment/$config_policy $config_policy=quay.io/open-cluster-management/config-policy-controller:$GRC_VERSION -n ${MANAGED_NAMESPACE}
  kubectl --context "$cluster" set env deployment/${config_policy} -n ${MANAGED_NAMESPACE} --containers=${config_policy} WATCH_NAMESPACE="${MANAGED_CLUSTER_NAME}"
  #kubectl patch deployment/${COMPONENT} -n ${MANAGED_NAMESPACE} --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--cluster-name='"${MANAGED_CLUSTER_NAME}"'"}]'
}

check_policy_readiness() {
  local hub="$1"
  local managed="$2"
  SECOND=0
  while true; do
    if [ $SECOND -gt 100 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    compCount=0

    # Deploy the policy framework hub controllers
    HUB_NAMESPACE="open-cluster-management"

    policyPropagator=$(kubectl get pods -n "${HUB_NAMESPACE}" --context "${hub}" --ignore-not-found | grep "governance-policy-propagator" || true)
    if [[ $(echo "${policyPropagator}" | awk '{print $3}') == "Running" ]]; then
      ((compCount = compCount + 1))
      echo "Policy: step${compCount} ${hub} ${policyPropagator} is Running"
    fi

    comp="governance-policy-spec-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      ((compCount = compCount + 1))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="governance-policy-status-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      ((compCount = compCount + 1))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="governance-policy-template-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      ((compCount = compCount + 1))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="config-policy-controller"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      ((compCount = compCount + 1))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    if [[ "${compCount}" == 5 ]]; then
      echo -e "Policy: ${hub} -> ${managed} Success! \n $(kubectl get pods -n "${MANAGED_NAMESPACE}")"
      break
    fi

    sleep 1
    ((SECOND = SECOND + 1))
  done
}

enable_router() {
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl --context "$1" apply -f -
  path="https://raw.githubusercontent.com/openshift/router/$ROUTE_VERSION"
  kubectl --context "$1" apply -f $path/deploy/route_crd.yaml
  # pacman application depends on route crd, but we do not need to have route pod running in the cluster
  # kubectl apply -f $path/deploy/router.yaml
  # kubectl apply -f $path/deploy/router_rbac.yaml
}

install_crds() {
  local ctx=$1
  local install_acm_crds=${2:-true}
  # router
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -
  path="https://raw.githubusercontent.com/openshift/router/$ROUTE_VERSION"
  kubectl --context "$ctx" apply -f $path/deploy/route_crd.yaml

  #proxy crd. required by olm
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_03_config-operator_01_proxies.crd.yaml

  # monitor
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_04_monitoring.coreos.com_servicemonitors.crd.yaml
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_04_monitoring.coreos.com_prometheusrules.yaml

  # clusterID from clusterversion
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/clusterversion.crd.yaml

  if [ "$install_acm_crds" = false ]; then
    return
  fi
  # mch
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml

  # clusterclaim: agent
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_02_clusters.open-cluster-management.io_clusterclaims.crd.yaml

  # clusterinfo for rest
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_06_internal.open-cluster-management.io_managedclusterinfos.crd.yaml

  # managedserviceaccount
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_06_authentication.open-cluster-management.io_managedserviceaccounts.crd.yaml

  # cluster managers
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_01_operator.open-cluster-management.io_clustermanagers.crd.yaml

  # addons
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_01_addon.open-cluster-management.io_managedclusteraddons.crd.yaml
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_00_addon.open-cluster-management.io_clustermanagementaddons.crd.yaml
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_02_addon.open-cluster-management.io_addondeploymentconfigs.crd.yaml

  # cluster
  kubectl --context "$ctx" apply -f "$CURRENT_DIR"/../manifest/crd/0000_00_cluster.open-cluster-management.io_managedclusters.crd.yaml
}

install_mch() {
  local ctx=$1
  # create open-cluster-management namespace
  kubectl create ns open-cluster-management --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -

  # mch
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/crd/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml

  # instance
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../manifest/mch/multiclusterhub.yaml

  # patch it to ready
  kubectl --context "$ctx" patch multiclusterhub multiclusterhub -n open-cluster-management --type='merge' -p='{"status": {"phase": "Running", "currentVersion": "2.13.1", "desiredVersion": "2.13.1"}}' --subresource=status
}

enable_service_ca() {
  local name=$1 # the name is the same with context
  local resource_dir=$2
  # apply service-ca
  kubectl --context "$name" label node "$name"-control-plane node-role.kubernetes.io/master=
  kubectl --context "$name" apply -f "$resource_dir"/service-ca-crds
  kubectl create ns openshift-config-managed --dry-run=client -o yaml | kubectl --context "$name" apply -f -
  kubectl --context "$name" apply -f "$resource_dir"/service-ca/
}

# deploy olm
enable_olm() {
  kubectl config use-context "$1"
  curl -L https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.28.0/install.sh -o install.sh
  chmod +x install.sh
  ./install.sh v0.28.0
}

wait_secret_ready() {
  secretName=$1
  secretNamespace=$2
  ready=$(kubectl get secret $secretName -n $secretNamespace --ignore-not-found=true)
  seconds=200
  while [[ $seconds -gt 0 && -z "$ready" ]]; do
    echo "wait secret: $secretNamespace - $secretName to be ready..."
    sleep 1
    seconds=$((seconds - 1))
    ready=$(kubectl get secret $secretName -n $secretNamespace --ignore-not-found=true)
  done
  if [[ $seconds == 0 ]]; then
    echo "failed(timeout) to create secret: $secretNamespace - $secretName!"
    exit 1
  fi
  echo "secret: $secretNamespace - $secretName is ready!"
}

wait_kafka_ready() {
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found)
  SECOND=0
  while [ -z "$clusterIsReady" ]; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for deploying kafka.kafka.strimzi.io/kafka"
      exit 1
    fi
    echo "Waiting for kafka cluster to become available"
    sleep 1
    ((SECOND = SECOND + 1))
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
  wait_secret_ready ${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"} "multicluster-global-hub"
  echo "Kafka secret is ready"
}

wait_postgres_ready() {
  clusterIsReady=$(kubectl -n hoh-postgres get PostgresCluster/hoh -o jsonpath={.status.instances..readyReplicas} --ignore-not-found)
  SECOND=0
  while [[ -z "$clusterIsReady" || "$clusterIsReady" -lt 1 ]]; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for deploying PostgresCluster/hoh"
      exit 1
    fi
    echo "Waiting for Postgres cluster to become available"
    sleep 1
    ((SECOND = SECOND + 1))
    clusterIsReady=$(kubectl -n hoh-postgres get PostgresCluster/hoh -o jsonpath={.status.instances..readyReplicas} --ignore-not-found)
  done
  echo "Postgres cluster is ready"
  wait_secret_ready ${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"} "multicluster-global-hub"
  echo "Postgres secret is ready"
}

wait_disappear() {
  command=$1
  seconds=${2:-"600"}
  while [ -n "$(eval $command)" ]; do
    if [ $seconds -lt 0 ]; then
      echo "timout for disappearing[$seconds]: $command"
      exit 1
    fi
    echo "waiting to disappear[$seconds]: $command"
    sleep 1
    ((seconds = seconds - 1))
  done
  echo "> $command"
  eval "$command"
}

wait_cmd() {
  local command=$1
  local seconds=${2:-"600"}
  local interval=2         # Interval for updating the waiting message
  local command_interval=4 # Interval for executing the command
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
      echo -e "\r placeholder will be overwrite by waitng message"
    fi
    local index=$((elapsed / interval % ${#signs[@]}))
    echo -ne "\r ${signs[$index]} Waiting $elapsed seconds: $1"
    sleep $interval
    ((elapsed += interval))
  done

  echo -e "\n$RED Timeout $seconds seconds $NC: $command"
  exit 1 # exit for fast failure
}

version_compare() {
  if [[ $1 == $2 ]]; then
    return 0
  fi
  local IFS=.
  local i version1=($1) version2=($2)
  # fill empty fields in version1 with zeros
  for ((i = ${#version1[@]}; i < ${#version2[@]}; i++)); do
    version1[i]=0
  done
  for ((i = 0; i < ${#version1[@]}; i++)); do
    if [[ -z ${version2[i]} ]]; then
      # fill empty fields in version2 with zeros
      version2[i]=0
    fi
    if ((10#${version1[i]} > 10#${version2[i]})); then
      return 1
    fi
    if ((10#${version1[i]} < 10#${version2[i]})); then
      return 2
    fi
  done

  return 0
}

check_golang() {
  export PATH=$PATH:/usr/local/go/bin
  if ! command -v go >/dev/null 2>&1; then
    wget https://dl.google.com/go/$GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf $GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm $GO_VERSION.linux-amd64.tar.gz
  fi
  if [[ $(go version) < "go version go1.23" ]]; then
    echo "go version is less than 1.23, update to $GO_VERSION"
    sudo rm -rf /usr/local/go
    wget https://dl.google.com/go/$GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf $GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm $GO_VERSION.linux-amd64.tar.gz
    sleep 2
  fi
  echo "go version: $(go version)"
}

check_ginkgo() {
  if ! command -v ginkgo >/dev/null 2>&1; then
    go install github.com/onsi/ginkgo/v2/ginkgo@$GINKGO_VERSION
    go get github.com/onsi/gomega/...
    sudo mv $(go env GOPATH)/bin/ginkgo $INSTALL_DIR/ginkgo
  fi
  echo "ginkgo version: $(ginkgo version)"
}

install_docker() {
  sudo yum install -y yum-utils device-mapper-persistent-data lvm2
  sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
  sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

  sleep 5
  sudo systemctl start docker
  sudo systemctl enable docker
}

check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    install_docker
  fi
  version_compare $(docker version --format '{{.Client.Version}}') "20.10.0"
  verCom=$?
  if [ $verCom -eq 2 ]; then
    # upgrade
    echo "remove old version of docker $(docker version --format '{{.Client.Version}}')"
    sudo yum remove -y docker docker-client docker-client-latest docker-common docker-latest docker-latest-logrotate docker-logrotate docker-selinux docker-engine-selinux docker-engine
    install_docker
  fi
  echo "docker version: $(docker version --format '{{.Client.Version}}')"
}

check_volume() {
  if [[ $(df -h | grep -v Size | awk '{print $2}' | sed -e 's/G//g' | awk 'BEGIN{ max = 0 } {if ($1 > max) max = $1; fi} END{print max}') -lt 80 ]]; then
    max_size=$(lsblk | awk '{print $1,$4}' | grep -v Size | grep -v M | sed -e 's/G//g' | awk 'BEGIN{ max = 0 } {if ($2 > max) max = $2; fi} END{print max}')
    mount_name=$(lsblk | grep "$max_size"G | awk '{print $1}')
    echo "mounting /dev/${mount_name}: ${max_size}"
    sudo mkfs -t xfs /dev/${mount_name}
    sudo mkdir -p /data
    sudo mount /dev/${mount_name} /data
    sudo mv /var/lib/docker /data

    # sudo sed -i "s/ExecStart=\/usr\/bin\/dockerd\ -H/ExecStart=\/usr\/bin\/dockerd\ -g\ \/data\/docker\ -H/g" /lib/systemd/system/docker.service
    echo '{
      "data-root": "/data/docker"
    }' | sudo tee /etc/docker/daemon.json

    sleep 2

    sudo systemctl daemon-reload
    sudo systemctl restart docker

    sudo docker info
  fi
  echo "docker root dir: $(docker info -f '{{ .DockerRootDir}}')"
}

enable_cluster() {
  local hub=$1
  local cluster="$2"
  # Apply label to managedcluster
  kubectl label mcl "$cluster" vendor=OpenShift --context "$hub" --overwrite 2>&1
  if ! kubectl --context "$cluster" get clusterclaim "id.k8s.io" >/dev/null 2>&1; then
    # Add clusterclaim
    cat <<EOF | kubectl --context "$cluster" apply -f -
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: ClusterClaim
metadata:
  labels:
    open-cluster-management.io/hub-managed: ""
    velero.io/exclude-from-backup: "true"
  name: id.k8s.io
spec:
  value: $(uuidgen)
EOF
  fi

  if ! kubectl --context "$cluster" get clusterversion version >/dev/null 2>&1; then
    # Add clusterversion
    cat <<EOF | kubectl --context "$cluster" apply -f -
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: stable-4.18
  clusterID: $(uuidgen)
EOF
  fi
}

wait_ocm() {
  local hub=$1
  local cluster=$2
  echo -e "$BLUE waiting OCM $1:$2 components $NC"
  kubectl wait deploy/cluster-manager -n open-cluster-management --for condition=Available=True --timeout=200s --context "$hub"
  kubectl get pod -n open-cluster-management-hub --context "$hub" # other hub controle planes

  kubectl wait deploy/klusterlet-registration-agent -n open-cluster-management-agent --for condition=Available=True --timeout=200s --context "$cluster"
  kubectl wait deploy/klusterlet-work-agent -n open-cluster-management-agent --for condition=Available=True --timeout=200s --context "$cluster"
}

wait_policy() {
  local hub=$1
  local cluster=$2
  echo -e "$BLUE waiting Policy $1:$2 components $NC"

  wait_cmd "kubectl get deploy/governance-policy-propagator -n open-cluster-management --context $hub"
  kubectl wait deploy/governance-policy-propagator -n open-cluster-management --for condition=Available=True --timeout=600s --context "$hub"

  kubectl get secret/hub-kubeconfig -n open-cluster-management-agent-addon --context "$cluster"
  wait_cmd "kubectl get deploy/governance-policy-framework-addon -n open-cluster-management-agent-addon --context $cluster"
  kubectl wait deploy/governance-policy-framework-addon -n open-cluster-management-agent-addon --for condition=Available=True --timeout=200s --context "$cluster"

  # configuration policy controller
  wait_cmd "kubectl get deploy/config-policy-controller -n open-cluster-management-agent-addon --context $cluster"
  kubectl wait deploy/config-policy-controller -n open-cluster-management-agent-addon --for condition=Available=True --timeout=200s --context "$cluster"
}

wait_application() {
  local hub=$1
  local cluster=$2
  echo -e "$BLUE waiting Application $1:$2 compoenents $NC"
  wait_cmd "kubectl get deploy/multicluster-operators-subscription -n open-cluster-management --context $hub"
  kubectl wait deploy/multicluster-operators-subscription -n open-cluster-management --for condition=Available=True --timeout=200s --context "$hub"

  wait_cmd "kubectl get deploy/application-manager -n open-cluster-management-agent-addon --context $cluster"
  kubectl wait deploy/application-manager -n open-cluster-management-agent-addon --for condition=Available=True --timeout=200s --context "$cluster"
}

# Common function to run a command with retries
# Parameters: $1 = command to run (function name or command string)
retry() {
  local retries=${2:-3}
  local count=0
  local success=false

  echo -e "${CYAN}$1 $NC "
  while [ $count -lt "$retries" ]; do
    echo -e "${YELLOW} Attempt $((count + 1))... $NC "
    if eval "$1"; then
      success=true
      break
    else
      ((count++))
      sleep 5 # Adjust the sleep duration as needed
    fi
  done

  if [ "$success" = true ]; then
    echo -e "${GREEN}$1: Success. $NC "
  else
    echo -e "${RED}$1: Failed after $retries attempts. $NC "
  fi
}

# Define ANSI escape codes for colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

BOLD_GREEN='\033[1;32m'
