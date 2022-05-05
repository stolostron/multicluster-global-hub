#!/bin/bash
#
# PREREQUISITE:
#  Docker
#  KinD v0.12.0  https://kind.sigs.k8s.io/docs/user/quick-start/ 
#  kubectl client
#  clusteradm client: MacOS M1 needs to install clusteradm in advance
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM
# NOTE:
#  the ocm_setup.sh script must be located under the same directory as the ocm_clean.sh 
#


HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-1}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-2}

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${CONFIG_DIR}/setup.log

if [ ! -d "${CONFIG_DIR}" ];then
  mkdir "${CONFIG_DIR}"
fi

if ! hash clusteradm > "${LOG}" 2>&1; then 
  curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
fi

function hover() {
  local pid=$1; message=${2:-Processing!}; delay=0.2
  echo "$pid" > "${CONFIG_DIR}/pid"
  signs=(🙉 🙈 🙊)
  while ( kill -0 "$pid" 2>/dev/null ); do
    index="${RANDOM} % ${#signs[@]}"
    printf "\e[38;5;$((RANDOM%257))m%s\r\e[0m" "[$(date '+%H:%M:%S')]  ${signs[${index}]}  ${message} ..."; sleep ${delay}
  done
  printf "%s\n" "[$(date '+%H:%M:%S')]  ✅  ${message} Done! "; sleep ${delay}
}

export KUBECONFIG="${CONFIG_DIR}/kubeconfig"
sleep 1 &
hover $! "export KUBECONFIG=${CONFIG_DIR}/kubeconfig"


function initCluster() {
  clusterName=$1
  kind create cluster --name "${clusterName}"
  SECOND=0
  while [[ $(kind get clusters | grep "^${clusterName}$") != "${clusterName}" ]]; do
    if [[ $SECOND -gt 12000 ]]; then
      echo "Timeout for creating cluster ${clusterName}."
      exit 1
    fi
    if [[ $((SECOND % 20)) == 0 ]]; then
      kind create cluster --name "${clusterName}"
    fi
    sleep 5
    (( SECOND = SECOND + 5 ))
  done
}

# initCluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initCluster "hub${i}" >> "${LOG}" 2>&1 & 
  hover $! "Cluster hub${i}" 
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initCluster "hub${i}-cluster${j}" >> "${LOG}" 2>&1 & 
    hover $! "Cluster hub${i}-cluster${j}" 
  done
done


function initHub() {
  hub="kind-${1}"
  kubectl config use-context "${hub}"
  clusteradm init --wait --context "${hub}" > "${CONFIG_DIR}/${hub}"
}

function initManaged() {
  hub="kind-${1}"
  managed="kind-${2}"
  if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managed}") == "" ]]; then
    kubectl config use-context "${managed}"
    clusteradm get token --context "${hub}" | grep "clusteradm" > "${CONFIG_DIR}/${hub}"
    sed -e "s;<cluster_name>;${managed} --force-internal-endpoint-lookup;" "${CONFIG_DIR}/${hub}" | bash
  fi
    
  SECOND=0
  while true; do
    if [ $SECOND -gt 12000 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    kubectl config use-context "${hub}"
    hubManagedCsr=$(kubectl get csr --context "${hub}" --ignore-not-found | grep "${managed}")
    if [[ "${hubManagedCsr}" != "" && $(echo "${hubManagedCsr}" | awk '{print $6}')  =~ "Pending" ]]; then 
      clusteradm accept --clusters "${managed}" --context "${hub}"
    fi

    hubManagedCluster=$(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managed}")
    if [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "true" ]]; then
      echo -e "Cluster ${managed} \n ${hubManagedCluster}"
      break
    elif [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "false" ]]; then
      kubectl patch managedcluster "${managed}" -p='{"spec":{"hubAcceptsClient":true}}' --type=merge --context "${hub}"
    else
      sleep 5
      (( SECOND = SECOND + 5 ))
    fi
  done
}


# init ocm
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initHub "hub${i}" >> "${LOG}" 2>&1 &
  hover $! "OCM hub${i}" 
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initManaged "hub${i}" "hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "OCM hub${i}-cluster${j}" 
  done
done


# init application-lifecycle
function initApp() {
  hub="kind-${1}"
  managed="kind-${2}"
  SECOND=0
  while true; do
    if [ $SECOND -gt 12000 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi
    
    # deploy the subscription operators to the hub cluster
    kubectl config use-context "${hub}"
    hubSubOperator=$(kubectl -n open-cluster-management get deploy multicluster-operators-subscription --context "${hub}" --ignore-not-found)
    if [[ "${hubSubOperator}" == "" ]]; then 
      clusteradm install hub-addon --names application-manager
      sleep 2
    fi

    # create ocm-agent-addon namespace on the managed cluster
    managedAgentAddonNS=$(kubectl get ns --context "${managed}" --ignore-not-found | grep "open-cluster-management-agent-addon")
    if [[ "${managedAgentAddonNS}" == "" ]]; then
      kubectl create ns open-cluster-management-agent-addon --context "${managed}"
    fi

    # deploy the the subscription add-on to the managed cluster
    hubManagedClusterAddon=$(kubectl -n "${managed}" get managedclusteraddon --context "${hub}" --ignore-not-found | grep application-manager)
    if [[ "${hubManagedClusterAddon}" == "" ]]; then
      kubectl config use-context "${hub}"
      clusteradm addon enable --name application-manager --cluster "${managed}"
    fi 

    managedSubAvailable=$(kubectl -n open-cluster-management-agent-addon get deploy --context "${managed}" --ignore-not-found | grep "application-manager")
    if [[ "${managedSubAvailable}" != "" && $(echo "${managedSubAvailable}" | awk '{print $4}') -gt 0 ]]; then
      echo -e "Application ${managed} \n $managedSubAvailable"
      break
    else
      sleep 10
      (( SECOND = SECOND + 10 ))
    fi
  done
}


# init app
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initApp "hub${i}" "hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "Application hub${i}-cluster${j}" 
  done
done

function initPolicy() {
  hub="kind-${1}"
  kindHub="${1}"
  managed="kind-${2}"
  SECOND=0
  while true; do
    if [ $SECOND -gt 12000 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    componentCount=0

    # Deploy the policy framework hub controllers
    kubectl config use-context "${hub}"
    HUB_NAMESPACE="open-cluster-management"
    HUB_KUBECONFIG=${CONFIG_DIR}/${hub}_kubeconfig
    
    kind get kubeconfig --name "${kindHub}" --internal > "${HUB_KUBECONFIG}"

    policyPropagator=$(kubectl get pods -n "${HUB_NAMESPACE}" --context "${hub}" --ignore-not-found | grep "governance-policy-propagator")
    if [[ ${policyPropagator} == "" ]]; then 
      kubectl create ns ${HUB_NAMESPACE} > /dev/null 2>&1
      GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io/governance-policy-propagator/main/deploy"
      ## Apply the CRDs
      kubectl apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policies.yaml
      kubectl apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_placementbindings.yaml 
      kubectl apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policyautomations.yaml
      kubectl apply -f ${GIT_PATH}/crds/policy.open-cluster-management.io_policysets.yaml
      ## Deploy the policy-propagator
      kubectl apply -f ${GIT_PATH}/operator.yaml -n ${HUB_NAMESPACE}
    elif [[ $(echo "${policyPropagator}" | awk '{print $3}')  == "Running" ]]; then 
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${hub} ${policyPropagator} is Running" 
    fi

    # Deploy the synchronization components to the managed cluster(s)
    kubectl config use-context "${managed}" 
    export MANAGED_NAMESPACE="open-cluster-management-agent-addon"
    export GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io"
    ## Create the namespace for the synchronization components
    kubectl create ns "${MANAGED_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

    ## Create the secret to authenticate with the hub
    if [[ $(kubectl get secret --context "${managed}" --ignore-not-found | grep "hub-kubeconfig") == "" ]]; then 
      kubectl -n "${MANAGED_NAMESPACE}" create secret generic hub-kubeconfig --from-file=kubeconfig="${HUB_KUBECONFIG}"
    fi
    ## Apply the policy CRD
    kubectl apply -f ${GIT_PATH}/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_policies.yaml 
    ## Set the managed cluster name and create the namespace
    kubectl create ns "${managed}" --dry-run=client -o yaml | kubectl apply -f -

    COMPONENT="governance-policy-spec-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}")
    if [[ ${comp} == "" ]]; then 
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}" 
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}" 
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-status-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}")
    if [[ ${comp} == "" ]]; then 
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}"
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}"
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-template-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}")
    if [[ ${comp} == "" ]]; then 
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}" 
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}"
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    if [[ "${componentCount}" == 4 ]]; then
      echo -e "Policy: ${hub} -> ${managed} Success! \n $(kubectl get pods -n "${MANAGED_NAMESPACE}")"
      break;
    fi 

    sleep 5
    (( SECOND = SECOND + 5 ))
  done
}

# init policy
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initPolicy "hub${i}" "hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "Policy hub${i}-cluster${j}" 
  done
done

printf "%s\033[0;32m%s\n\033[0m " "[Access the Clusters]: " "export KUBECONFIG=${CONFIG_DIR}/kubeconfig"