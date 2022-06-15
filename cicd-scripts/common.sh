set -e

function checkEnv() {
  name="$1"
  value="$2"
  if [[ -z "${value}" ]]; then
    echo "Error: environment variable $name must be specified!"
    exit 1
  fi
}

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

function initPolicy() {
  hub="$1"
  managed="$2"
  HUB_KUBECONFIG="$3"
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

    sleep 5
    (( SECOND = SECOND + 5 ))
  done
}