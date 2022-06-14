#!/bin/bash

# PREREQUISITE:
#  Docker 
#  KinD v0.12.0  https://kind.sigs.k8s.io/docs/user/quick-start/ 
#  kubectl client
#  clusteradm client: MacOS M1 needs to install clusteradm in advance

source ./ocm_setup.sh

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${CONFIG_DIR}/setup_hoh.log

if [ ! -d "${CONFIG_DIR}" ];then
  mkdir "${CONFIG_DIR}"
fi

KUBECONFIG=${CONFIG_DIR}/kubeconfig

LEAF_HUB_NAME="kind-hub1"
HUB_OF_HUB_NAME="acm-hub-of-hubs"

CTX_HUB="microshift"
CTX_MANAGED="kind-hub1"

if [ ! -d "${CONFIG_DIR}" ];then
  mkdir "${CONFIG_DIR}"
fi

# install clusteradm
if ! hash clusteradm > "${LOG}" 2>&1; then 
  curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
fi

function hover() {
  local pid=$1; message=${2:-Processing!}; delay=0.2
  echo "$pid" > "${CONFIG_DIR}/pid_hoh"
  signs=(🙉 🙈 🙊)
  while ( kill -0 "$pid" 2>/dev/null ); do
    index="${RANDOM} % ${#signs[@]}"
    printf "\e[38;5;$((RANDOM%257))m%s\r\e[0m" "[$(date '+%H:%M:%S')]  ${signs[${index}]}  ${message} ..."; sleep ${delay}
  done
  printf "%s\n" "[$(date '+%H:%M:%S')]  ✅  ${message} Done! "; sleep ${delay}
}

sleep 1 &
hover $! "export KUBECONFIG=${KUBECONFIG}"

function initMicroshiftCluster() {
  if [[ $(docker ps | grep "${HUB_OF_HUB_NAME}") == "" ]]; then
    # create microshift cluster
    docker run -d --rm --name $HUB_OF_HUB_NAME --privileged -v hub-data:/var/lib -p 6443:6443 quay.io/microshift/microshift-aio:4.8.0-0.microshift-2022-04-20-182108
    sleep 5
    docker cp ${HUB_OF_HUB_NAME}:/var/lib/microshift/resources/kubeadmin/kubeconfig ${KUBECONFIG}
    echo "docker cp ${HUB_OF_HUB_NAME}:/var/lib/microshift/resources/kubeadmin/kubeconfig ${KUBECONFIG}"
    sleep 5
    kubectl config use-context microshift

    # enable the applications.app.k8s.io
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml
    # enable the route.openshift.io
    kubectl create ns openshift-ingress 
    kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/route_crd.yaml
    kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/router.yaml
    kubectl apply -f https://raw.githubusercontent.com/openshift/router/master/deploy/router_rbac.yaml
  fi
}

function initLeafHubCluster() {
  if [[ $(kubectl config get-contexts | grep "${LEAF_HUB_NAME}") == "" ]]; then
    bash ${CURRENT_DIR}/ocm_setup.sh
  fi
}

# init clusters
initMicroshiftCluster >> $LOG 2>&1 &
hover $! "HoH create micronshift cluster"

initLeafHubCluster >> $LOG 2>&1 &
hover $! "HoH create leaf hub cluster"

function initHub() {
  CTX=$1
  clusteradm init --wait --context $CTX > "${CONFIG_DIR}/join_hoh"
}

function initManaged() {
  CTX_HUB=$1
  CTX_MANAGED=$2
  if [[ $(kubectl get managedcluster --context "$CTX_HUB" --ignore-not-found | grep "$CTX_MANAGED") == "" ]]; then
    kubectl config use-context "$CTX_HUB"
    clusteradm get token --context "$CTX_HUB" | grep "clusteradm" > "${CONFIG_DIR}/join_hoh"

    # deploy a klusterlet agent on the managed cluster
    export kubectl config use-context $CTX_MANAGED
    sed -e "s;<cluster_name>;${CTX_MANAGED} --context $CTX_MANAGED;" "${CONFIG_DIR}/join_hoh" > "${CONFIG_DIR}/join_hoh_name"
    CONTAINER_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' acm-hub-of-hubs) 
    sed -e "s;127.0.0.1;${CONTAINER_IP};" "${CONFIG_DIR}/join_hoh_name" > "${CONFIG_DIR}/join_hoh_name_ip"
    cat ${CONFIG_DIR}/join_hoh_name_ip | bash
  fi

  SECOND=0
  while true; do
    if [ $SECOND -gt 12000 ]; then
      echo "Timeout waiting for ${CTX_HUB} + ${CTX_MANAGED}."
      exit 1
    fi

    kubectl config use-context $CTX_HUB
    hubManagedCsr=$(kubectl get csr --context $CTX_HUB --ignore-not-found | grep "$CTX_MANAGED")
    if [[ "${hubManagedCsr}" != "" && $(echo "${hubManagedCsr}" | awk '{print $5}')  =~ "Pending" ]]; then 
      clusteradm accept --clusters "$CTX_MANAGED" --context "$CTX_HUB"
    fi

    hubManagedCluster=$(kubectl get managedcluster --context "$CTX_HUB" --ignore-not-found | grep "$CTX_MANAGED")
    if [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "true" ]]; then
      echo -e "Cluster $CTX_HUB : ${CTX_MANAGED}"
      break
    elif [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "false" ]]; then
      kubectl patch managedcluster "$CTX_MANAGED" -p='{"spec":{"hubAcceptsClient":true}}' --type=merge --context "$CTX_HUB"
    else
      sleep 5
      (( SECOND = SECOND + 5 ))
    fi
  done
}

# deloy ocm on hub of hubs 
initHub $CTX_HUB >> $LOG 2>&1 &
hover $! "HoH deploy a cluster manager on the hub cluster"

initManaged $CTX_HUB $CTX_MANAGED >> $LOG 2>&1 &
hover $! "HoH deploy a klusterlet agent on the leaf hub"


# add application to hub of hubs
initApp $CTX_HUB $CTX_MANAGED >> "${LOG}" 2>&1 &
hover $! "HoH Application $CTX_HUB - $CTX_MANAGED" 

# add policy to hub of hubs
HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig_hoh_manager
kubectl config view --context=${hub} --minify --flatten > ${HUB_KUBECONFIG}
## or docker cp ${HUB_OF_HUB_NAME}:/var/lib/microshift/resources/kubeadmin/kubeconfig ${KUBECONFIG}
CONTAINER_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' acm-hub-of-hubs) 
sed -e "s;127.0.0.1;${CONTAINER_IP};" "${HUB_KUBECONFIG}" > "${HUB_KUBECONFIG}_ip"
HUB_KUBECONFIG="${HUB_KUBECONFIG}_ip"

initPolicy $CTX_HUB $CTX_MANAGED $HUB_KUBECONFIG >> "${LOG}" 2>&1 &
hover $! "HoH Policy $CTX_HUB - $CTX_MANAGED" 

printf "%s\033[0;32m%s\n\033[0m " "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"
