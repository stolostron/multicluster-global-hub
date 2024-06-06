#!/bin/bash

export INSTALL_DIR=/usr/bin
export GRC_VERSION=v0.13.0
export KUBECTL_VERSION=v1.28.1
export CLUSTERADM_VERSION=0.8.2
export KIND_VERSION=v0.23.0
export ROUTE_VERSION=release-4.12
export GO_VERSION=go1.21.7
export GINKGO_VERSION=v2.15.0
export CONFIG_GRC_VERSION=v0.13.0

function check_dir() {
  if [ ! -d "$1" ];then
    mkdir "$1"
  fi
}

function hover() {
  local pid=$1; message=${2:-Processing!}; delay=0.4
  current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  echo "$pid" > "${current_dir}/config/pid"
  signs=(🙉 🙈 🙊)
  while ( kill -0 "$pid" 2>/dev/null ); do
    if [[ $LOG =~ "/dev/"* ]]; then
      sleep $delay
    else
      index="${RANDOM} % ${#signs[@]}"
      printf "\e[38;5;$((RANDOM%257))m%s\r\e[0m" "[$(date '+%H:%M:%S')]  ${signs[${index}]}  ${message} ..."; sleep ${delay}
    fi
  done
  printf "%s\n" "[$(date '+%H:%M:%S')]  ✅  ${message}";
}

function check_kubectl() {
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

function check_clusteradm() {
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

function check_kind() {
  if ! command -v kind >/dev/null 2>&1; then 
    echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
    curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-$(uname)-amd64"
    chmod +x ./kind-amd64
    sudo mv ./kind-amd64 $INSTALL_DIR/kind
  fi
}

function kind_cluster() {
  cluster_name="$1"
  if ! kind get clusters | grep -q "^$cluster_name$"; then
    kind create cluster --name "$cluster_name" --wait 1m
    dir="${KUBE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
    kubectl config view --context="kind-$cluster_name" --minify --flatten > "$dir/kind-$cluster_name"
  fi
}

function init_hub() {
  echo "Initializing Hub $1 ..."
  clusteradm init --wait --context "$1" > /dev/null 2>&1
  kubectl wait deployment -n open-cluster-management cluster-manager --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-controller --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-webhook --for condition=Available=True --timeout=200s  --context "$1"
  dir="${KUBE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  join_file="$dir/join-$1"
  clusteradm get token --context "$1" | grep "clusteradm" > "$join_file"
}

function init_managed() {
  hub="$1"
  managed_prefix_name="$2"
  managed_cluster_num="$3"
  managed_cluster_name=""

  dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  join_cmd="${dir}/config/join-${hub}"
  for i in $(seq 1 "${managed_cluster_num}"); do
    managed="${managed_prefix_name}$i"
    if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "$managed" || true) == "" ]]; then
      kubectl config use-context "$managed"
      clusteradm get token --context "${hub}" | grep "clusteradm" > "$join_cmd"
      if [[ $hub =~ "kind" ]]; then
        sed -e "s;<cluster_name>;$managed --force-internal-endpoint-lookup --wait;" "$join_cmd" > "${join_cmd}-named"
        sed -e "s;<cluster_name>;$managed --force-internal-endpoint-lookup --wait;" "$join_cmd" | bash
      else
        sed -e "s;<cluster_name>;$managed --wait;" "$join_cmd" > "${join_cmd}-named"
        sed -e "s;<cluster_name>;$managed --wait;" "$join_cmd" | bash
      fi
    fi
    managed_cluster_name+="$managed,"
  done
  clusteradm accept --clusters "${managed_cluster_name%,*}" --context "${hub}" --wait
}

function join_cluster() {
  hub=$1 # hub name also as the context
  cluster=$2
  echo "join cluster: $cluster to $hub"
  dir="${KUBE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  join_file="$dir/join-$1"
  if ! kubectl get mcl --context "${hub}" --ignore-not-found | grep -q "^$cluster$"; then
    if [[ $hub =~ "kind" ]]; then
      sed -e "s;<cluster_name>;$cluster --force-internal-endpoint-lookup --context $cluster --wait;" "$join_file" | bash
    else
      sed -e "s;<cluster_name>;$cluster --context $cluster --wait;" "$join_file" | bash
    fi
  fi
  clusteradm accept --clusters "$2" --context "${hub}" --wait
}

# init application-lifecycle
function init_app() {
  echo "init app for $1:$2"
  hub=$1
  cluster=$2

   # Use other branch throw error: unable to read URL "https://raw.githubusercontent.com/kubernetes-sigs/application/v0.8.0/deploy/kube-app-manager-aio.yaml", server reported 404 Not Found, status code=404
  app_path=https://raw.githubusercontent.com/kubernetes-sigs/application/master
  kubectl apply -f $app_path/deploy/kube-app-manager-aio.yaml --context "$hub"
  kubectl apply -f $app_path/deploy/kube-app-manager-aio.yaml --context "$cluster"

  # deploy the subscription operators to the hub cluster
  clusteradm install hub-addon --names application-manager --context "$hub"

  # enable the addon on the managed clusters
  clusteradm addon enable --names application-manager --clusters "$cluster" --context "$hub"
}

function init_policy() {
  echo "init policy for $1:$2"
  hub=$1
  cluster=$2

  dir="${KUBE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  HUB_KUBECONFIG="$dir/$hub"

  # create namespace fist
  HUB_NAMESPACE="open-cluster-management"
  kubectl create ns "${HUB_NAMESPACE}" --dry-run=client -o yaml | kubectl --context $hub apply -f -
  MANAGED_NAMESPACE="open-cluster-management-agent-addon"
  kubectl create ns "${MANAGED_NAMESPACE}" --dry-run=client -o yaml | kubectl --context "$cluster" apply -f -

  # Reference: https://open-cluster-management.io/getting-started/integration/policy-framework/
  GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io/governance-policy-propagator/$GRC_VERSION/deploy"
  policyCRD=${GIT_PATH}/crds/policy.open-cluster-management.io_policies.yaml

  # On hub
  if ! kubectl --context $hub get deploy -n "$HUB_NAMESPACE" | grep -q governance-policy-propagator; then 
    ## Apply the CRDs
    kubectl --context $hub apply -f $policyCRD
    kubectl --context $hub apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_placementbindings.yaml
    kubectl --context $hub apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policyautomations.yaml
    kubectl --context $hub apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policysets.yaml
    # Deploy the policy-propagator
    kubectl --context $hub apply -f ${GIT_PATH}/operator.yaml -n ${HUB_NAMESPACE}
    ## Disable the webhook for the controller
    kubectl --context $hub patch deployment governance-policy-propagator --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--enable-webhooks=false"}]' -n ${HUB_NAMESPACE}
    kubectl --context $hub patch deployment governance-policy-propagator --type='json' -p='[
    {"op": "remove", "path": "/spec/template/spec/containers/0/volumeMounts/0"},
    {"op": "remove", "path": "/spec/template/spec/volumes/0"} 
    ]' -n ${HUB_NAMESPACE}
  fi

  # On cluster
  ## Create the secret to authenticate with the hub
  if [[ $(kubectl get secret hub-kubeconfig -n "${MANAGED_NAMESPACE}" --context "$cluster" --ignore-not-found) == "" ]]; then 
    kubectl --context "$cluster" -n "${MANAGED_NAMESPACE}" create secret generic hub-kubeconfig --from-file=kubeconfig="$HUB_KUBECONFIG"
  fi

  ## Apply the policy CRD
  kubectl --context "$cluster" apply -f "$policyCRD"

  ## Deploy the synchronization component
  COMPONENT=governance-policy-framework-addon
  GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io/$COMPONENT/$GRC_VERSION"
  DEPLOY_ON_HUB=false # Set whether or not this is being deployed on the Hub
  MANAGED_CLUSTER_NAME="$cluster" # Set the managed cluster name and create the namespace
  kubectl create ns "$MANAGED_CLUSTER_NAME" --dry-run=client -o yaml | kubectl --context "$cluster" apply -f -

  kubectl --context "$cluster" apply -f ${GIT_PATH}/deploy/operator.yaml -n ${MANAGED_NAMESPACE}
  kubectl --context "$cluster" set image deployment/$COMPONENT $COMPONENT=quay.io/open-cluster-management/governance-policy-framework-addon:$GRC_VERSION -n ${MANAGED_NAMESPACE}
  kubectl --context "$cluster" patch deployment $COMPONENT -n ${MANAGED_NAMESPACE} \
    -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"governance-policy-framework-addon\",\"args\":[\"--hub-cluster-configfile=/var/run/klusterlet/kubeconfig\", \"--cluster-namespace=${MANAGED_CLUSTER_NAME}\", \"--enable-lease=true\", \"--log-level=2\", \"--disable-spec-sync=${DEPLOY_ON_HUB}\"]}]}}}}"

  ## Install configuration policy controller:
  ## https://open-cluster-management.io/getting-started/integration/policy-controllers/configuration-policy/ 
  COMPONENT="config-policy-controller"
  GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io/${COMPONENT}/$CONFIG_GRC_VERSION/deploy"
  kubectl --context "$cluster" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_configurationpolicies.yaml
  # kubectl --context "$cluster" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_operatorpolicies.yaml
  kubectl --context "$cluster" apply -f ${GIT_PATH}/operator.yaml -n ${MANAGED_NAMESPACE}
  kubectl --context "$cluster" set image deployment/$COMPONENT $COMPONENT=quay.io/open-cluster-management/config-policy-controller:$CONFIG_GRC_VERSION -n ${MANAGED_NAMESPACE}
  kubectl set env deployment/${COMPONENT} -n ${MANAGED_NAMESPACE} --containers=${COMPONENT} WATCH_NAMESPACE="${MANAGED_CLUSTER_NAME}"
}

function check_policy_readiness() {
  hub="$1"
  managed="$2"
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
    if [[ $(echo "${policyPropagator}" | awk '{print $3}')  == "Running" ]]; then 
      (( compCount = compCount + 1 ))
      echo "Policy: step${compCount} ${hub} ${policyPropagator} is Running" 
    fi

    comp="governance-policy-spec-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( compCount = compCount + 1 ))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="governance-policy-status-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( compCount = compCount + 1 ))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="governance-policy-template-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( compCount = compCount + 1 ))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    comp="config-policy-controller"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${comp}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( compCount = compCount + 1 ))
      echo "Policy: step${compCount} ${managed} ${comp} is Running"
    fi

    if [[ "${compCount}" == 5 ]]; then
      echo -e "Policy: ${hub} -> ${managed} Success! \n $(kubectl get pods -n "${MANAGED_NAMESPACE}")"
      break;
    fi 

    sleep 1
    (( SECOND = SECOND + 1 ))
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
  # router
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -
  path="https://raw.githubusercontent.com/openshift/router/$ROUTE_VERSION"
  kubectl --context "$ctx" apply -f $path/deploy/route_crd.yaml
 
  # mch
  kubectl --context "$ctx" apply -f ${CURRENT_DIR}/../../pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml
}

enable_service_ca() {
  ctx=$1
  node_name=$2
  resource_dir=$3
  # apply service-ca
  kubectl --context "$ctx" label node "$node_name"-control-plane node-role.kubernetes.io/master=
  kubectl --context "$ctx" apply -f "$resource_dir"/service-ca-crds
  kubectl --context "$ctx" create ns openshift-config-managed
  kubectl --context "$ctx" apply -f "$resource_dir"/service-ca/
}

# deploy olm
function enable_olm() {
  NS=olm
  csvPhase=$(kubectl --context "$1" get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  if [[ "$csvPhase" == "Succeeded" ]]; then
    echo "OLM is already installed in ${NS} namespace. Exiting..."
    exit 1
  fi
  
  path="https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/v0.22.0"
  kubectl --context "$1" apply -f "${path}/deploy/upstream/quickstart/crds.yaml"
  kubectl --context "$1" wait --for=condition=Established -f "${path}/deploy/upstream/quickstart/crds.yaml" --timeout=60s
  kubectl --context "$1" apply -f "${path}/deploy/upstream/quickstart/olm.yaml"

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

function wait_secret_ready() {
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

function wait_kafka_ready() {
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found)
  SECOND=0
  while [ -z "$clusterIsReady" ]; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for deploying kafka.kafka.strimzi.io/kafka"
      exit 1
    fi
    echo "Waiting for kafka cluster to become available"
    sleep 1
    (( SECOND = SECOND + 1 ))
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
  wait_secret_ready ${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"} "multicluster-global-hub"
  echo "Kafka secret is ready"
}

function wait_postgres_ready() {
  clusterIsReady=$(kubectl -n hoh-postgres get PostgresCluster/hoh -o jsonpath={.status.instances..readyReplicas} --ignore-not-found)
  SECOND=0
  while [[ -z "$clusterIsReady" || "$clusterIsReady" -lt 1 ]]; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for deploying PostgresCluster/hoh"
      exit 1
    fi
    echo "Waiting for Postgres cluster to become available"
    sleep 1
    (( SECOND = SECOND + 1 ))
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
    (( seconds = seconds - 1 ))
  done
  echo "> $command"
  eval $command
}

wait_appear() {
  command=$1
  seconds=${2:-"600"}
  while [ -z "$(eval $command)" ]; do 
    if [ $seconds -lt 0 ]; then
      echo -e "$RED timout [$seconds] $NC: $command"
      exit 1
    fi 
    echo -e "$YELLOW waiting [$seconds] $NC: $command"
    sleep 5
    (( seconds = seconds - 5 ))
  done
  echo "> $command"
  eval $command
}

function version_compare() {
  if [[ $1 == $2 ]]
  then
    return 0
  fi
  local IFS=.
  local i version1=($1) version2=($2)
  # fill empty fields in version1 with zeros
  for ((i=${#version1[@]}; i<${#version2[@]}; i++))
  do
    version1[i]=0
  done
  for ((i=0; i<${#version1[@]}; i++))
  do
    if [[ -z ${version2[i]} ]]
    then
      # fill empty fields in version2 with zeros
      version2[i]=0
    fi
    if ((10#${version1[i]} > 10#${version2[i]}))
    then
      return 1
    fi
    if ((10#${version1[i]} < 10#${version2[i]}))
    then
      return 2
    fi
  done

  return 0
}

function check_golang() {
  export PATH=$PATH:/usr/local/go/bin
  if ! command -v go >/dev/null 2>&1; then
    wget https://dl.google.com/go/$GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf $GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm $GO_VERSION.linux-amd64.tar.gz
  fi
  if [[ $(go version) < "go version go1.20" ]]; then
    echo "go version is less than 1.20, update to $GO_VERSION"
    sudo rm -rf /usr/local/go
    wget https://dl.google.com/go/$GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf $GO_VERSION.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm $GO_VERSION.linux-amd64.tar.gz
    sleep 2
  fi
  echo "go version: $(go version)"
}

function check_ginkgo() {
  if ! command -v ginkgo >/dev/null 2>&1; then 
    go install github.com/onsi/ginkgo/v2/ginkgo@$GINKGO_VERSION
    go get github.com/onsi/gomega/...
    sudo mv $(go env GOPATH)/bin/ginkgo $INSTALL_DIR/ginkgo
  fi 
  echo "ginkgo version: $(ginkgo version)"
}

function install_docker() {
  sudo yum install -y yum-utils device-mapper-persistent-data lvm2
  sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
  sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

  sleep 5
  sudo systemctl start docker
  sudo systemctl enable docker
}

function check_docker() {
  if ! command -v docker >/dev/null 2>&1; then 
    install_docker
  fi
  version_compare $(docker version --format '{{.Client.Version}}') "20.10.0"
  verCom=$?
  if [ $verCom -eq 2 ]; then
    # upgrade
    echo "remove old version of docker $(docker version --format '{{.Client.Version}}')"
    sudo yum remove -y docker docker-client docker-client-latest docker-common docker-latest docker-latest-logrotate docker-logrotate docker-selinux  docker-engine-selinux docker-engine 
    install_docker
  fi
  echo "docker version: $(docker version --format '{{.Client.Version}}')"
}

function check_volume() {
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
  hub=$1
  cluster="$2"
  # Apply label to managedcluster
  kubectl label mcl "$cluster" vendor=OpenShift --context "$hub" --overwrite 2>&1
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
}

# Define ANSI escape codes for colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color