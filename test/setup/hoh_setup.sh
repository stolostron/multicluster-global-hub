# !/bin/bash

set -o nounset

echo "using kubeconfig $KUBECONFIG"

namespace=open-cluster-management
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi

function deployConfigResources() {
  # apply the HoH config CRD
  configDir=${currentDir}/hoh/hub-of-hubs-config
  kubectl apply -f "${configDir}/crd.yaml"
  kubectl wait --for=condition=Established -f "${configDir}/crd.yaml"

  # create namespace if not exists
  kubectl create namespace hoh-system --dry-run=client -o yaml | kubectl apply -f -

  # apply default HoH config CR
  isCrReady=$(kubectl get configs.hub-of-hubs.open-cluster-management.io hub-of-hubs-config -n hoh-system --ignore-not-found)
  while [[ -z "$isCrReady" ]]; do
    sleep 5
    kubectl apply -f ${configDir}/cr.yaml
    isCrReady=$(kubectl get configs.hub-of-hubs.open-cluster-management.io hub-of-hubs-config -n hoh-system --ignore-not-found)
  done
  echo "HoH config resource is ready!"
}

function deployKafkaTransport() {
  kafkaNamespace="kafka"
  kubectl apply -f ${currentDir}/hoh/hub-of-hubs-kafka-topics.yaml
  isSpecReady=$(kubectl get kafkatopic spec -n $kafkaNamespace --ignore-not-found | grep spec)
  while [[ -z "$isSpecReady" ]]; do
    sleep 5
    isSpecReady=$(kubectl get kafkatopic spec -n $kafkaNamespace --ignore-not-found | grep spec)
  done
  isStatusReady=$(kubectl get kafkatopic status -n $kafkaNamespace --ignore-not-found | grep status)
  while [[ -z "$isStatusReady" ]]; do
    sleep 5
    isStatusReady=$(kubectl get kafkatopic status -n $kafkaNamespace --ignore-not-found | grep status)
  done
  echo "HoH kafka topic spec and status is ready!"
}

function deployController() {
  kubectl delete secret hub-of-hubs-database-secret -n "$namespace" --ignore-not-found
  kubectl create secret generic hub-of-hubs-database-secret -n "$namespace" --from-literal=url="$DATABASE_URL_HOH"

  kubectl delete secret hub-of-hubs-database-transport-bridge-secret -n "$namespace" --ignore-not-found
  kubectl create secret generic hub-of-hubs-database-transport-bridge-secret -n "$namespace" --from-literal=url="$DATABASE_URL_TRANSPORT"
  echo "created database secrets"

  export TRANSPORT_TYPE=${TRANSPORT_TYPE:-"kafka"}
  export REGISTRY=quay.io/open-cluster-management-hub-of-hubs
  export IMAGE_TAG="$TAG"
  envsubst < ${currentDir}/hoh/hub-of-hubs-manager.yaml | kubectl apply -f - -n "$namespace"
  kubectl wait deployment -n "$namespace" hub-of-hubs-manager --for condition=Available=True --timeout=600s
  echo "created hub-of-hubs-manager"

  # skip hub cluster controller on the test
  export ENFORCE_HOH_RBAC=${ENFORCE_HOH_RBAC:-"false"}
  CONTAINER_ID=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' hub-of-hubs-control-plane)
  export KAFKA_BOOTSTRAP_SERVER="${CONTAINER_ID}/30095"
  envsubst < ${currentDir}/hoh/hub-of-hubs-addon.yaml | kubectl apply -f - -n "$namespace"
  kubectl wait deployment -n "$namespace" hub-of-hubs-addon-controller --for condition=Available=True --timeout=600s
  echo "created hub-of-hubs-addon"

  echo "HoH controller is ready!"
}

function initPostgres() {
  pgNamespace="hoh-postgres"
  processUser="hoh-pguser-hoh-process-user"
  transportUser="hoh-pguser-transport-bridge-user"
  DATABASE_URL_HOH="$(kubectl get secrets -n "${pgNamespace}" "${processUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
  DATABASE_URL_TRANSPORT="$(kubectl get secrets -n "${pgNamespace}" "${transportUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"

  kubectl delete -f ${currentDir}/hoh/hub-of-hubs-postgres-job.yaml --ignore-not-found=true
  export IMAGE=quay.io/open-cluster-management-hub-of-hubs/postgresql-ansible:$TAG
  envsubst < ${currentDir}/hoh/hub-of-hubs-postgres-job.yaml | kubectl apply -f -
  kubectl wait --for=condition=complete job/postgres-init -n $pgNamespace --timeout=600s
  kubectl logs $(kubectl get pods --field-selector status.phase=Succeeded  --selector=job-name=postgres-init -n $pgNamespace  --output=jsonpath='{.items[*].metadata.name}') -n $pgNamespace
}

function deployRbac() {
  kubectl delete secret opa-data -n "$namespace" --ignore-not-found
  secretDir=${currentDir}/hoh/hub-of-hubs-rbac/secret
  kubectl create secret generic opa-data -n "$namespace" --from-file=${secretDir}/data.json --from-file=${secretDir}/role_bindings.yaml --from-file=${secretDir}/opa_authorization.rego
  echo "created rbac secret opt-data"

  export REGISTRY=quay.io/open-cluster-management-hub-of-hubs
  export IMAGE_TAG="$TAG"
  export COMPONENT=hub-of-hubs-rbac
  envsubst < ${currentDir}/hoh/hub-of-hubs-rbac/operator.yaml | kubectl apply -f - -n "$namespace"
  kubectl wait deployment -n "$namespace" "$COMPONENT" --for condition=Available=True --timeout=600s
  echo "created rbac operator"

  # update mutating webhook configuration to inject identity to policies + placementbidnings
  if [[ ! -z $(kubectl get mutatingwebhookconfiguration ocm-mutating-webhook --ignore-not-found) ]]; then
    kubectl get mutatingwebhookconfiguration ocm-mutating-webhook -o json \
      | jq --argjson rules_patch '{"apiGroups": ["policy.open-cluster-management.io"], "apiVersions": ["v1"], "operations": ["CREATE"], "resources": ["policies", "placementbindings"], "scope": "*"}' '.webhooks[0].rules += [$rules_patch]' \
      | jq 'del(.metadata.managedFields, .metadata.resourceVersion, .metadata.generation, .metadata.creationTimestamp)' \
      | kubectl apply -f -
  fi
  echo "HoH rbac is ready!"
}

function patchImages() {

  # update policy image
  kubectl patch deployment governance-policy-propagator -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"governance-policy-propagator","image":"quay.io/open-cluster-management-hub-of-hubs/governance-policy-propagator:hub-of-hubs"}]}}}}'

  # update app image
  kubectl patch deployment multicluster-operators-placementrule -n open-cluster-management -p '{"spec":{"template":{"spec":{"containers":[{"name":"multicluster-operators-placementrule","image":"quay.io/open-cluster-management-hub-of-hubs/multicloud-operators-subscription:hub-of-hubs"}]}}}}'

  # update the cluster-manager palacement image
  kubectl apply -f ${currentDir}/hoh/hub-of-hubs-cluster-manager.yaml
 
  echo "HoH images is updated!"
}

deployConfigResources
deployKafkaTransport
initPostgres
export DATABASE_URL_HOH=$DATABASE_URL_HOH
export DATABASE_URL_TRANSPORT=$DATABASE_URL_TRANSPORT
echo "export DATABASE_URL_HOH=$DATABASE_URL_HOH"
echo "export DATABASE_URL_TRANSPORT=$DATABASE_URL_TRANSPORT"
deployRbac
deployController
patchImages