#!/bin/bash

# PREREQUISITE:
#  Docker 
#  KinD v0.12.0  https://kind.sigs.k8s.io/docs/user/quick-start/ 
#  kubectl client
#  clusteradm client: MacOS M1 needs to install clusteradm in advance

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${CONFIG_DIR}/setup_hoh.log

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
    docker run -d --rm --name $HUB_OF_HUB_NAME --privileged -v hub-data:/var/lib -p 6443:6443 quay.io/microshift/microshift-aio:latest
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
    wget -O - https://github.com/stolostron/hub-of-hubs/blob/release-2.5/demos/setup/ocm_setup.sh | bash
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


# init application-lifecycle
function initApp() {
  hub="$1"
  managed="$2"
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
      sleep 5
      (( SECOND = SECOND + 5 ))
    fi
  done
}

initApp $CTX_HUB $CTX_MANAGED >> "${LOG}" 2>&1 &
hover $! "HoH Application $CTX_HUB - $CTX_MANAGED" 

function initPolicy() {
  hub="$1"
  managed="$2"
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
    HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig_hoh_manager
    
    # deploy policy propagator in the hub cluster
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
    kubectl config view --context=${hub} --minify --flatten > ${HUB_KUBECONFIG}
    # or docker cp ${HUB_OF_HUB_NAME}:/var/lib/microshift/resources/kubeadmin/kubeconfig ${KUBECONFIG}
    CONTAINER_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' acm-hub-of-hubs) 
    sed -e "s;127.0.0.1;${CONTAINER_IP};" "${HUB_KUBECONFIG}" > "${HUB_KUBECONFIG}_ip"
    HUB_KUBECONFIG="${HUB_KUBECONFIG}_ip"
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

    COMPONENT="config-policy-controller"
    # Apply the config-policy-controller CRD
    kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/crds/policy.open-cluster-management.io_configurationpolicies.yaml
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}")
    if [[ ${comp} == "" ]]; then
      # Deploy the controller
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}"
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}"
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    if [[ "${componentCount}" == 5 ]]; then
      echo -e "Policy: ${hub} -> ${managed} Success! \n $(kubectl get pods -n "${MANAGED_NAMESPACE}")"
      break;
    fi 

    sleep 5
    (( SECOND = SECOND + 5 ))
  done
}

initPolicy $CTX_HUB $CTX_MANAGED >> "${LOG}" 2>&1 &
hover $! "HoH Policy $CTX_HUB - $CTX_MANAGED" 

printf "%s\033[0;32m%s\n\033[0m " "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"
