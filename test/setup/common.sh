
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
  clusterName=$1
  image="kindest/node:v1.23.6@sha256:b1fa224cc6c7ff32455e0b1fd9cbfd3d3bc87ecaa8fcb06961ed1afb3db0f9ae" # or kindest/node:v1.23.4 
  if [[ $(kind get clusters | grep "^${clusterName}$" || true) != "${clusterName}" ]]; then
    kind create cluster --name "$clusterName" --image "$image" --wait 1m
    currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    kubectl config view --context="kind-${clusterName}" --minify --flatten > ${currentDir}/config/kubeconfig-${clusterName}
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
  echo "Initializing Hub $1 ..."
  clusteradm init --wait --context "$1" > /dev/null 2>&1
  kubectl wait deployment -n open-cluster-management cluster-manager --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-controller --for condition=Available=True --timeout=200s --context "$1" 
  kubectl wait deployment -n open-cluster-management-hub cluster-manager-registration-webhook --for condition=Available=True --timeout=200s  --context "$1"
}

function initManaged() {
  hub="$1"
  managed="$2"

  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  joinCommand="${currentDir}/config/join-${hub}"
  if [[ $(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managed}" || true) == "" ]]; then
    kubectl config use-context "${managed}"
    clusteradm get token --context "${hub}" | grep "clusteradm" > "$joinCommand"
    if [[ $hub =~ "kind" ]]; then
      sed -e "s;<cluster_name>;${managed} --force-internal-endpoint-lookup --wait;" "$joinCommand" > "${joinCommand}-named"
      sed -e "s;<cluster_name>;${managed} --force-internal-endpoint-lookup --wait;" "$joinCommand" | bash
    else
      sed -e "s;<cluster_name>;${managed} --wait;" "$joinCommand" > "${joinCommand}-named"
      sed -e "s;<cluster_name>;${managed} --wait;" "$joinCommand" | bash
    fi
    sleep 2
    kubectl scale deployment klusterlet -n open-cluster-management --replicas=1
  fi
    
  SECOND=0
  while true; do
    if [ $SECOND -gt 300 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    kubectl config use-context "${hub}"
    hubManagedCsr=$(kubectl get csr --context "${hub}" --ignore-not-found | grep "${managed}" || true)
    while [[ "${hubManagedCsr}" != "" && "${hubManagedCsr}" =~ "Pending" ]]; do
      echo "accepting ${hub} + ${managed} csr"
      clusteradm accept --clusters "${managed}" --context "${hub}"
      sleep 5
      (( SECOND = SECOND + 5 ))
      hubManagedCsr=$(kubectl get csr --context "${hub}" --ignore-not-found | grep "${managed}" || true)
    done

    hubManagedCluster=$(kubectl get managedcluster --context "${hub}" --ignore-not-found | grep "${managed}" || true)
    if [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "true" ]]; then
      echo "Cluster ${managed}: ${hubManagedCluster}"
      break
    elif [[ "${hubManagedCluster}" != "" && $(echo "${hubManagedCluster}" | awk '{print $2}') == "false" ]]; then
      echo "Cluster ${managed}: ${hubManagedCluster}"
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
    if [ $SECOND -gt 300 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi
    
    # deploy the subscription operators to the hub cluster
    kubectl config use-context "${hub}"
    hubSubOperator=$(kubectl -n open-cluster-management get deploy multicluster-operators-subscription --context "${hub}" --ignore-not-found)
    if [[ -z "${hubSubOperator}" ]]; then 
      echo "$hub install hub-addon application-manager"
      clusteradm install hub-addon --names application-manager
      sleep 2
    fi

    # create ocm-agent-addon namespace on the managed cluster
    managedAgentAddonNS=$(kubectl get ns --context "${managed}" --ignore-not-found | grep "open-cluster-management-agent-addon" || true)
    if [[ -z "${managedAgentAddonNS}" ]]; then
      echo "create namespace open-cluster-management-agent-addon on ${managed}"
      kubectl create ns open-cluster-management-agent-addon --context "${managed}"
    fi

    echo "check the application-manager is available on context: ${managed} "
    managedSubAvailable=$(kubectl -n open-cluster-management-agent-addon get deploy --context "${managed}" --ignore-not-found | grep "application-manager" || true)
    if [[ "${managedSubAvailable}" != "" && $(echo "${managedSubAvailable}" | awk '{print $4}') -gt 0 ]]; then
      echo "Application installed $hub - ${managed}: $managedSubAvailable"
      break
    else
      echo "Deploying the the subscription add-on to the managed cluster: $managed"
      kubectl config use-context "${hub}"
      clusteradm addon enable --name application-manager --cluster "${managed}"
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
    if [ $SECOND -gt 400 ]; then
      echo "Timeout waiting for ${hub} + ${managed}."
      exit 1
    fi

    componentCount=0

    # Deploy the policy framework hub controllers
    kubectl config use-context "${hub}"
    HUB_NAMESPACE="open-cluster-management"

    policyPropagator=$(kubectl get pods -n "${HUB_NAMESPACE}" --context "${hub}" --ignore-not-found | grep "governance-policy-propagator" || true)
    if [[ ${policyPropagator} == "" ]]; then 
      kubectl create ns "${HUB_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
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
    if [[ $(kubectl get secret hub-kubeconfig -n "${MANAGED_NAMESPACE}" --context "${managed}" --ignore-not-found) == "" ]]; then 
      kubectl -n "${MANAGED_NAMESPACE}" create secret generic hub-kubeconfig --from-file=kubeconfig="${HUB_KUBECONFIG}"
    fi

    ## Apply the policy CRD
    kubectl apply -f ${GIT_PATH}/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_policies.yaml 

    ## Set the managed cluster name and create the namespace
    kubectl create ns "${managed}" --dry-run=client -o yaml | kubectl apply -f -

    COMPONENT="governance-policy-spec-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ ${comp} == "" ]]; then 
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}" 
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}" 
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-status-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
    if [[ ${comp} == "" ]]; then 
      kubectl apply -f ${GIT_PATH}/"${COMPONENT}"/main/deploy/operator.yaml -n "${MANAGED_NAMESPACE}"
      kubectl set env deployment/"${COMPONENT}" -n "${MANAGED_NAMESPACE}" --containers="${COMPONENT}" WATCH_NAMESPACE="${managed}"
    elif [[ $(echo "${comp}" | awk '{print $3}') == "Running" ]]; then
      (( componentCount = componentCount + 1 ))
      echo "Policy: step${componentCount} ${managed} ${COMPONENT} is Running"
    fi

    COMPONENT="governance-policy-template-sync"
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
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
    comp=$(kubectl get pods -n "${MANAGED_NAMESPACE}" --ignore-not-found | grep "${COMPONENT}" || true)
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

enableRouter() {
  kubectl config use-context "$1"
  kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl apply -f -
  GIT_PATH="https://raw.githubusercontent.com/openshift/router/release-4.12"
  kubectl apply -f $GIT_PATH/deploy/route_crd.yaml
  # pacman application depends on route crd, but we do not need to have route pod running in the cluster
  # kubectl apply -f $GIT_PATH/deploy/router.yaml
  # kubectl apply -f $GIT_PATH/deploy/router_rbac.yaml
}

function enableDependencyResources() {
  kubectl config use-context "$1"
  # crd
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  kubectl apply -f ${currentDir}/crds

  # # router: the microshift has the router resources - will be deprecated later
  # kubectl create ns openshift-ingress --dry-run=client -o yaml | kubectl apply -f -
  # GIT_PATH="https://raw.githubusercontent.com/openshift/router/release-4.12"
  # kubectl apply -f $GIT_PATH/deploy/route_crd.yaml
  # kubectl apply -f $GIT_PATH/deploy/router.yaml
  # kubectl apply -f $GIT_PATH/deploy/router_rbac.yaml

  # # service ca: the microshift has the service ca resources - will be deprecated later
  # kubectl create ns openshift-config-managed --dry-run=client -o yaml | kubectl apply -f -
  # kubectl apply -f ${currentDir}/service-ca
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
  kubectl wait --for=condition=Established -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml" --timeout=60s
  kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"

  retries=60
  csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  while [[ $retries -gt 0 && "$csvPhase" != "Succeeded" ]]; do
    echo "csvPhase: ${csvPhase}"
    sleep 5
    retries=$((retries - 1))
    csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  done
  kubectl rollout status -w deployment/packageserver --namespace="${NS}" --timeout=60s

  if [ $retries == 0 ]; then
    echo "CSV \"packageserver\" failed to reach phase succeeded"
    exit 1
  fi
  echo "CSV \"packageserver\" install succeeded"
}

function connectMicroshift() {
  invokeContainerName=$1
  microshiftContainerName=$2
  invokeNetwork=$(docker inspect -f '{{range $key, $value := .NetworkSettings.Networks}}{{$key}} {{end}}' $invokeContainerName)
  microshiftContainerNetwork=$(docker inspect -f '{{range $key, $value := .NetworkSettings.Networks}}{{$key}} {{end}}' $microshiftContainerName)
  if [[ "$invokeNetwork" =~ "$microshiftContainerNetwork" ]]; then
    echo "Microshift network is already connected to ${invokeNetwork}"
    exit 1
  else 
    docker network connect $microshiftContainerNetwork $invokeContainerName
  fi
}

function waitSecretToBeReady() {
  secretName=$1
  secretNamespace=$2
  ready=$(kubectl get secret $secretName -n $secretNamespace --ignore-not-found=true)
  seconds=200
  while [[ $seconds -gt 0 && -z "$ready" ]]; do
    echo "wait secret: $secretNamespace - $secretName to be ready..."
    sleep 5
    seconds=$((seconds - 5))
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
    sleep 5
    (( SECOND = SECOND + 5 ))
    clusterIsReady=$(kubectl -n kafka get kafka.kafka.strimzi.io/kafka-brokers-cluster -o jsonpath={.status.listeners} --ignore-not-found)
  done
  echo "Kafka cluster is ready"
  waitSecretToBeReady ${TRANSPORT_SECRET_NAME:-"transport-secret"} "open-cluster-management"
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
    sleep 5
    (( SECOND = SECOND + 5 ))
    clusterIsReady=$(kubectl -n hoh-postgres get PostgresCluster/hoh -o jsonpath={.status.instances..readyReplicas} --ignore-not-found)
  done
  echo "Postgres cluster is ready"
  waitSecretToBeReady ${STORAGE_SECRET_NAME:-"storage-secret"} "open-cluster-management"
  echo "Postgres secret is ready"
}

function checkManagedCluster() {
  context=$1
  cluster=$2
  available=$(kubectl get mcl --context "$context" | grep "$cluster" |awk '{print $5}')
  seconds=100
  while [[ "$available" != "True" && $seconds -gt 0 ]]; do
    echo "retry to import managed cluster $cluster to $context"
    initManaged $context $cluster
    available=$(kubectl get mcl --context "$context" | grep "$cluster" |awk '{print $5}')
    sleep 5
    seconds=$((seconds - 5))
  done
  if [[ "$available" != "True" ]]; then 
    echo "failed to import managed cluster $cluster to $context"
    exit 1
  fi
}