#!/bin/bash
function checkEnv() {
  name="$1"
  value="$2"
  if [[ -z "${value}" ]]; then
    echo "Error: environment variable $name must be specified!"
    exit 1
  fi
}

function checkDir() {
  if [ ! -d "$1" ];then
    mkdir "$1"
  fi
}

function hover() {
  local pid=$1; message=${2:-Processing!}; delay=0.4
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  echo "$pid" > "${currentDir}/config/pid"
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

function checkKubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then 
      echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
      if [[ "$(uname)" == "Linux" ]]; then
          curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/linux/amd64/kubectl
      elif [[ "$(uname)" == "Darwin" ]]; then
          curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/darwin/amd64/kubectl
      fi
      chmod +x ./kubectl
      sudo mv ./kubectl /usr/local/bin/kubectl
  fi
}

function checkClusteradm() {
  if ! hash clusteradm 2>/dev/null; then 
    curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
  fi
}

function checkKind() {
  if ! command -v kind >/dev/null 2>&1; then 
    echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
    curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/v0.12.0/kind-$(uname)-amd64"
    chmod +x ./kind-amd64
    sudo mv ./kind-amd64 /usr/local/bin/kind
  fi
}

function initKinDCluster() {
  clusterName="$1"
  kindConfig="$2"
  if [[ $(kind get clusters | grep "^${clusterName}$" || true) != "${clusterName}" ]]; then
    if [[ "x${kindConfig}" == "x" ]]; then
      kind create cluster --name "$clusterName" --wait 1m
    else
      kind create cluster --name "$clusterName" --wait 1m --config $kindConfig
    fi
    currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    kubectl config view --context="kind-${clusterName}" --minify --flatten > ${currentDir}/config/kubeconfig-${clusterName}
  fi
}

function initHub() {
  echo "Initializing Hub $1 ..."
  clusteradm init --wait --context "$1" > /dev/null 2>&1
  kubectl wait deployment -n open-cluster-management cluster-manager --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-controller --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-webhook --for condition=Available=True --timeout=200s  --context "$1"
}

function initManaged() {
  hub="$1"
  managedPrefix="$2"
  managedClusterNum="$3"
  managedClusterName=""

  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  joinCommand="${currentDir}/config/join-${hub}"
  for i in $(seq 1 "${managedClusterNum}"); do
    if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managedPrefix}$i" || true) == "" ]]; then
      kubectl config use-context "${managedPrefix}$i"
      clusteradm get token --context "${hub}" | grep "clusteradm" > "$joinCommand"
      if [[ $hub =~ "kind" ]]; then
        sed -e "s;<cluster_name>;${managedPrefix}$i --force-internal-endpoint-lookup --wait;" "$joinCommand" > "${joinCommand}-named"
        sed -e "s;<cluster_name>;${managedPrefix}$i --force-internal-endpoint-lookup --wait;" "$joinCommand" | bash
      else
        sed -e "s;<cluster_name>;${managedPrefix}$i --wait;" "$joinCommand" > "${joinCommand}-named"
        sed -e "s;<cluster_name>;${managedPrefix}$i --wait;" "$joinCommand" | bash
      fi
    fi
    managedClusterName+="${managedPrefix}$i,"
  done
  clusteradm accept --clusters "${managedClusterName%,*}" --context "${hub}" --wait
}

# init application-lifecycle
function initApp() {
  echo "init app for $1"
  hub="$1"
  managedPrefix="$2"
  managedClusterNum="$3"

  # enable the applications.app.k8s.io
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml --context "${hub}"

  for i in $(seq 1 "${managedClusterNum}"); do    
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml --context "${managedPrefix}$i"
    # deploy the subscription operators to the hub cluster
    echo "$hub install hub-addon application-manager"
    clusteradm install hub-addon --names application-manager --context "${hub}"

    echo "Deploying the the subscription add-on to the managed cluster: $managedPrefix$i"
    clusteradm addon enable --names application-manager --clusters "${managedPrefix}$i" --context "${hub}"
  done
}

function initPolicy() {
  echo "init policy for $1"
  hub="$1"
  managedPrefix="$2"
  managedClusterNum="$3"
  HUB_KUBECONFIG="$4"

  # Deploy the policy framework hub controllers
  HUB_NAMESPACE="open-cluster-management"

  for i in $(seq 1 "${managedClusterNum}"); do
    kubectl create ns "${HUB_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io/governance-policy-propagator/v0.11.0/deploy"
    ## Apply the CRDs
    kubectl --context "${hub}" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policies.yaml 
    kubectl --context "${hub}" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_placementbindings.yaml 
    kubectl --context "${hub}" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policyautomations.yaml
    kubectl --context "${hub}" apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policysets.yaml
    ## Deploy the policy-propagator
    kubectl --context "${hub}" apply -f ${GIT_PATH}/operator.yaml -n ${HUB_NAMESPACE}
    kubectl --context "${hub}" patch deployment governance-policy-propagator -n ${HUB_NAMESPACE} -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management/governance-policy-propagator:v0.11.0"}]}}}}'

    # Deploy the synchronization components to the managed cluster(s)
    MANAGED_NAMESPACE="open-cluster-management-agent-addon"
    GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io"

    ## Create the namespace for the synchronization components
    kubectl create ns "${MANAGED_NAMESPACE}" --dry-run=client -o yaml | kubectl --context "${managedPrefix}$i" apply -f -

    ## Create the secret to authenticate with the hub
    if [[ $(kubectl get secret hub-kubeconfig -n "${MANAGED_NAMESPACE}" --context "${managedPrefix}$i" --ignore-not-found) == "" ]]; then 
      kubectl --context "${managedPrefix}$i" -n "${MANAGED_NAMESPACE}" create secret generic hub-kubeconfig --from-file=kubeconfig="${HUB_KUBECONFIG}"
    fi

    ## Apply the policy CRD
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_policies.yaml 

    ## Set the managed cluster name and create the namespace
    kubectl create ns "${managedPrefix}$i" --dry-run=client -o yaml | kubectl --context "${managedPrefix}$i" apply -f -

    COMPONENT="governance-policy-spec-sync"
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}" 
    kubectl --context "${managedPrefix}$i" set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managedPrefix}$i" 

    COMPONENT="governance-policy-status-sync"
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}"
    kubectl --context "${managedPrefix}$i" set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managedPrefix}$i"

    COMPONENT="governance-policy-template-sync"
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}" 
    kubectl --context "${managedPrefix}$i" set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managedPrefix}$i"

    COMPONENT="config-policy-controller"
    # Apply the config-policy-controller CRD
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/crds/policy.open-cluster-management.io_configurationpolicies.yaml
    # Deploy the controller
    kubectl --context "${managedPrefix}$i" apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}"
    kubectl --context "${managedPrefix}$i" set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managedPrefix}$i"
    
  done

}

function checkPolicyReadiness() {
  hub="$1"
  managed="$2"
  SECOND=0
  while true; do
    if [ $SECOND -gt 100 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    componentCount=0

    # Deploy the policy framework hub controllers
    HUB_NAMESPACE="open-cluster-management"

    policyPropagator=$(kubectl get pods -n "${HUB_NAMESPACE}" --context "${hub}" --ignore-not-found | grep "governance-policy-propagator" || true)
    if [[ $(echo "${policyPropagator}" | awk '{print $3}')  == "Running" ]]; then 
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${hub} ${policyPropagator} is Running" 
    fi

    COMPONENT="governance-policy-spec-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-status-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-template-sync"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="config-policy-controller"
    comp=$(kubectl --context "${managed}" get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    if [[ "${componentCount}" == 5 ]]; then
      echo -e "Policy: ${hub} -> ${managed} Success! \n $(kubectl get pods -n "${MANAGED_NAMESPACE}")"
      break;
    fi 

    sleep 1
    (( SECOND = SECOND + 1 ))
  done
}

enableRouter() {
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl --context "$1" apply -f -
  GIT_PATH="https://raw.githubusercontent.com/openshift/router/release-4.12"
  kubectl --context "$1" apply -f $GIT_PATH/deploy/route_crd.yaml
  # pacman application depends on route crd, but we do not need to have route pod running in the cluster
  # kubectl apply -f $GIT_PATH/deploy/router.yaml
  # kubectl apply -f $GIT_PATH/deploy/router_rbac.yaml
}

enableServiceCA() {
  HUB_OF_HUB_NAME=$2
  CURRENT_DIR=$3
  # apply service-ca
  kubectl --context $1 label node ${HUB_OF_HUB_NAME}-control-plane node-role.kubernetes.io/master=
  kubectl --context $1 apply -f ${CURRENT_DIR}/hoh/service-ca-crds
  kubectl --context $1 create ns openshift-config-managed
  kubectl --context $1 apply -f ${CURRENT_DIR}/hoh/service-ca/
}

# deploy olm
function enableOLM() {
  NS=olm
  csvPhase=$(kubectl --context "$1" get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  if [[ "$csvPhase" == "Succeeded" ]]; then
    echo "OLM is already installed in ${NS} namespace. Exiting..."
    exit 1
  fi
  
  GIT_PATH="https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/v0.22.0"
  kubectl --context "$1" apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  kubectl --context "$1" wait --for=condition=Established -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml" --timeout=60s
  kubectl --context "$1" apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"

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

function waitSecretToBeReady() {
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

function waitKafkaToBeReady() {
  clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  SECOND=0
  while [ -z "$clusterIsReady" ]; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for deploying kafka.kafka.strimzi.io/kafka-brokers-cluster"
      exit 1
    fi
    echo "Waiting for kafka cluster to become available"
    sleep 1
    (( SECOND = SECOND + 1 ))
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
  waitSecretToBeReady ${TRANSPORT_SECRET_NAME:-"multicluster-global-hub-transport"} "open-cluster-management"
  echo "Kafka secret is ready"
}

function waitPostgresToBeReady() {
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
  waitSecretToBeReady ${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"} "open-cluster-management"
  echo "Postgres secret is ready"
}

waitDisappear() {
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

waitAppear() {
  command=$1
  seconds=${2:-"600"}
  while [ -z "$(eval $command)" ]; do 
    if [ $seconds -lt 0 ]; then
      echo "timout for appearing[$seconds]: $command"
      exit 1
    fi 
    echo "waiting to appear[$seconds]: $command"
    sleep 1
    (( seconds = seconds - 1 ))
  done
  echo "> $command"
  eval $command
}