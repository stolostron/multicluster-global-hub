
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
  local pid=$1; message=${2:-Processing!}; delay=0.2
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  echo "$pid" > "${currentDir}/config/pid"
  signs=(🙉 🙈 🙊)
  while ( kill -0 "$pid" 2>/dev/null ); do
    index="${RANDOM} % ${#signs[@]}"
    printf "\e[38;5;$((RANDOM%257))m%s\r\e[0m" "[$(date '+%H:%M:%S')]  ${signs[${index}]}  ${message} ..."; sleep ${delay}
  done
  printf "%s\n" "[$(date '+%H:%M:%S')]  ✅  ${message}"; sleep ${delay}
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
  clusterName=$1
  image="kindest/node:v1.23.6@sha256:b1fa224cc6c7ff32455e0b1fd9cbfd3d3bc87ecaa8fcb06961ed1afb3db0f9ae" # or kindest/node:v1.23.4 
  if [[ $(kind get clusters | grep "^${clusterName}$") != "${clusterName}" ]]; then
    [ ! -n "$2" ] && (kind create cluster --name "$clusterName" --image "$image")  
    [ -n "$2" ] && (kind create cluster --name "$clusterName" --image "$image" --config $2)                      
  fi
}

function initMicroShift() {
  portMappings=$2
  portMappingArray=(${portMappings//;/ })
  portMappingFlag=""
  for portMap in "${portMappingArray[@]}"; do
    portMappingFlag="${portMappingFlag} -p ${portMap}"
  done
  docker run -d --rm --name $1 --privileged -v $1-data:/var/lib ${portMappingFlag} quay.io/microshift/microshift-aio:latest
}

function getMicroShiftKubeConfig() {
  containerIP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $1)

  # wait until the kubeconfig is copied to target file successfully
  until docker cp $1:/var/lib/microshift/resources/kubeadmin/kubeconfig $2 > /dev/null 2>&1
  do
    sleep 10
  done

  sed -i "s/microshift/${1}/" $2
  sed -i "s/: user/: ${1}/" $2
  sed -i "s/127.0.0.1/${containerIP}/" $2
}

function initHub() {
  hubCtx="$1"
  hubInitFile="$2"
  kubectl config use-context "$hubCtx"
  clusteradm init --wait --context "$hubCtx" > $hubInitFile
}

function initManaged() {
  hub="$1"
  managed="$2"
  hubInitFile="$3"
  if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managed}") == "" ]]; then
    kubectl config use-context "${managed}"
    clusteradm get token --context "${hub}" | grep "clusteradm" > "$hubInitFile"
    sed -e "s;<cluster_name>;${managed} --force-internal-endpoint-lookup;" "$hubInitFile" | bash
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
      echo "Cluster ${managed}: ${hubManagedCluster}"
      break
    elif [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "false" ]]; then
      kubectl patch managedcluster "${managed}" -p='{"spec":{"hubAcceptsClient":true}}' --type=merge --context "${hub}"
    fi
    sleep 5
    (( SECOND = SECOND + 5 ))
  done
}

# init application-lifecycle
function initApp() {
  hub="$1"
  managed="$2"
  echo "init application: $1 - $2"
  # enable the applications.app.k8s.io
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml --context "${hub}"
  kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/application/master/deploy/kube-app-manager-aio.yaml --context "${managed}"

  SECOND=0
  while true; do
    if [ $SECOND -gt 600 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi
    
    # deploy the subscription operators to the hub cluster
    kubectl config use-context "${hub}"
    hubSubOperator=$(kubectl -n open-cluster-management get deploy multicluster-operators-subscription --context "${hub}" --ignore-not-found)
    if [[ "${hubSubOperator}" == "" ]]; then 
      echo "$hub install hub-addon application-manager"
      clusteradm install hub-addon --names application-manager
      sleep 2
    fi

    # create ocm-agent-addon namespace on the managed cluster
    managedAgentAddonNS=$(kubectl get ns --context "${managed}" --ignore-not-found | grep "open-cluster-management-agent-addon")
    if [[ "${managedAgentAddonNS}" == "" ]]; then
      echo "create namespace open-cluster-management-agent-addon on ${managed}"
      kubectl create ns open-cluster-management-agent-addon --context "${managed}"
    fi

    # deploy the the subscription add-on to the managed cluster
    hubManagedClusterAddon=$(kubectl -n "${managed}" get managedclusteraddon --context "${hub}" --ignore-not-found | grep application-manager)
    if [[ "${hubManagedClusterAddon}" == "" ]]; then
      kubectl config use-context "${hub}"
      echo "deploy the the subscription add-on to the managed cluster: $hub - $managed"
      clusteradm addon enable --name application-manager --cluster "${managed}"
    fi 

    managedSubAvailable=$(kubectl -n open-cluster-management-agent-addon get deploy --context "${managed}" --ignore-not-found | grep "application-manager")
    if [[ "${managedSubAvailable}" != "" && $(echo "${managedSubAvailable}" | awk '{print $4}') -gt 0 ]]; then
      echo "Application installed $hub - ${managed}: $managedSubAvailable"
      break
    else
      sleep 5
      (( SECOND = SECOND + 5 ))
    fi
  done
}

function initPolicy() {
  echo "init policy"
  hub="$1"
  managed="$2"
  HUB_KUBECONFIG="$3"
  SECOND=0
  while true; do
    if [ $SECOND -gt 1200 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    componentCount=0

    # Deploy the policy framework hub controllers
    kubectl config use-context "${hub}"
    HUB_NAMESPACE="open-cluster-management"

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
    MANAGED_NAMESPACE="open-cluster-management-agent-addon"
    GIT_PATH="https://raw.githubusercontent.com/open-cluster-management-io"

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

    sleep 10
    (( SECOND = SECOND + 10 ))
  done
}

function enableDependencyResources() {
  kubectl config use-context "$1"
  # crd
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  kubectl apply -f ${currentDir}/crds

  # router
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl apply -f -
  GIT_PATH="https://raw.githubusercontent.com/openshift/router/release-4.12"
  kubectl apply -f $GIT_PATH/deploy/route_crd.yaml
  kubectl apply -f $GIT_PATH/deploy/router.yaml
  kubectl apply -f $GIT_PATH/deploy/router_rbac.yaml

  # service ca
  kubectl create ns openshift-config-managed --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f ${currentDir}/service-ca
}

# deploy olm
function enableOLM() {
  kubectl config use-context "$1"
  NS=olm
  csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  if [[ "$csvPhase" == "Succeeded" ]]; then
    echo "OLM is already installed in ${NS} namespace. Exiting..."
    exit 1
  fi
  
  GIT_PATH="https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/v0.21.2"
  kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  kubectl wait --for=condition=Established -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  sleep 10
  kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"

  retries=60
  until [[ $retries == 0 ]]; do
    csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
    if [[ "$csvPhase" == "Succeeded" ]]; then
      break
    fi
    kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
    kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"
    echo "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
    echo "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"
    sleep 20
    retries=$((retries - 1))

  done
  kubectl rollout status -w deployment/packageserver --namespace="${NS}" 

  if [ $retries == 0 ]; then
    echo "CSV \"packageserver\" failed to reach phase succeeded"
    exit 1
  fi
  echo "CSV \"packageserver\" install succeeded"
}
