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

source ${CURRENT_DIR}/common.sh

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
  kind create cluster --name "${clusterName}" --image kindest/node:v1.23.4
  SECOND=0
  while [[ $(kind get clusters | grep "^${clusterName}$") != "${clusterName}" ]]; do
    if [[ $SECOND -gt 12000 ]]; then
      echo "Timeout for creating cluster ${clusterName}."
      exit 1
    fi
    if [[ $((SECOND % 20)) == 0 ]]; then
      kind create cluster --name "${clusterName}" --image kindest/node:v1.23.4
    fi
    sleep 5
    (( SECOND = SECOND + 5 ))
  done
  # enable the applications.app.k8s.io
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml
  # enable the route.openshift.io
  kubectl create ns openshift-ingress 
  kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/route_crd.yaml
  kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/router.yaml
  kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/router_rbac.yaml
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

# init app
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initApp "kind-hub${i}" "kind-hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "Application hub${i}-cluster${j}" 
  done
done

# init policy
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  HUB_KUBECONFIG=${CONFIG_DIR}/hub${i}_kubeconfig
  kind get kubeconfig --name "hub${i}" --internal > "$HUB_KUBECONFIG"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initPolicy "kind-hub${i}" "kind-hub${i}-cluster${j}" "$HUB_KUBECONFIG" >> "${LOG}" 2>&1 &
    hover $! "Policy hub${i}-cluster${j}" 
  done
done

printf "%s\033[0;32m%s\n\033[0m " "[Access the Clusters]: " "export KUBECONFIG=${CONFIG_DIR}/kubeconfig"