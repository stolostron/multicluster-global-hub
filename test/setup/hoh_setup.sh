# !/bin/bash

set -euo pipefail

branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi
export OPENSHIFT_CI=${OPENSHIFT_CI:-"false"}
export MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF=${MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF:-"quay.io/open-cluster-management-hub-of-hubs/multicluster-globalhub-manager:$TAG"}
export MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF=${MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF:-"quay.io/open-cluster-management-hub-of-hubs/multicluster-globalhub-agent:$TAG"}

echo "KUBECONFIG $KUBECONFIG"
echo "OPENSHIFT_CI: $OPENSHIFT_CI"
echo "MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF $MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF"
echo "MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF $MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF"

namespace=open-cluster-management
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

function deployConfigResources() {
  # apply the HoH config CRD
  configDir=${currentDir}/hoh/multicluster-globalhub-config
  # create namespace if not exists
  kubectl create namespace hoh-system --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f ${configDir}/hoh-config.yaml
  echo "HoH config resource is ready!"
}

function deployKafkaTransport() {
  kafkaNamespace="kafka"
  kubectl apply -f ${currentDir}/hoh/multicluster-globalhub-kafka-topics.yaml
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
  kubectl delete secret multicluster-globalhub-database-secret -n "$namespace" --ignore-not-found
  kubectl create secret generic multicluster-globalhub-database-secret -n "$namespace" --from-literal=url="$DATABASE_URL_HOH"

  kubectl delete secret multicluster-globalhub-database-transport-bridge-secret -n "$namespace" --ignore-not-found
  kubectl create secret generic multicluster-globalhub-database-transport-bridge-secret -n "$namespace" --from-literal=url="$DATABASE_URL_TRANSPORT"
  echo "created database secrets"

  export TRANSPORT_TYPE=${TRANSPORT_TYPE:-"kafka"}
  envsubst < ${currentDir}/hoh/multicluster-globalhub-manager.yaml | kubectl apply -f - -n "$namespace"
  kubectl wait deployment -n "$namespace" multicluster-globalhub-manager --for condition=Available=True --timeout=600s
  echo "created multicluster-globalhub-manager"

  # skip hub cluster controller on the test

  # deploy multicluster-globalhub-addon component with environment variables
  export ENFORCE_HOH_RBAC=${ENFORCE_HOH_RBAC:-"false"}
  component="multicluster-globalhub-addon"
  rm -rf $component
  git clone https://github.com/stolostron/$component.git
  cd $component
  git checkout $branch
  mv ./deploy/deployment.yaml ./deploy/deployment.yaml.tmpl
  envsubst < ./deploy/deployment.yaml.tmpl > ./deploy/deployment.yaml
  kubectl apply -n "$namespace" -k ./deploy
  rm ./deploy/deployment.yaml.tmpl
  cd ..
  rm -rf $component
  kubectl wait deployment -n "$namespace" multicluster-globalhub-addon-controller --for condition=Available=True --timeout=600s
  echo "created multicluster-globalhub-addon"

  echo "HoH controller is ready!"
}

function initPostgres() {
  pgNamespace="hoh-postgres"
  pgUserSecretName="postgresql-secret"
  DATABASE_URL_HOH="$(kubectl get secrets -n "${namespace}" "${pgUserSecretName}" -o go-template='{{index (.data) "database_uri" | base64decode}}')"
  DATABASE_URL_TRANSPORT="$(kubectl get secrets -n "${namespace}" "${pgUserSecretName}" -o go-template='{{index (.data) "database_uri" | base64decode}}')"

  kubectl delete -f ${currentDir}/hoh/multicluster-globalhub-postgres-job.yaml --ignore-not-found=true
  export IMAGE=quay.io/open-cluster-management-hub-of-hubs/postgresql-ansible:$TAG
  envsubst < ${currentDir}/hoh/multicluster-globalhub-postgres-job.yaml | kubectl apply -f -
  kubectl wait --for=condition=complete job/postgres-init -n $pgNamespace --timeout=800s
  kubectl logs $(kubectl get pods --field-selector status.phase=Succeeded  --selector=job-name=postgres-init -n $pgNamespace  --output=jsonpath='{.items[*].metadata.name}') -n $pgNamespace
}

function deployRbac() {
  kubectl delete secret opa-data -n "$namespace" --ignore-not-found
  secretDir=${currentDir}/hoh/multicluster-globalhub-rbac/secret
  kubectl create secret generic opa-data -n "$namespace" --from-file=${secretDir}/data.json --from-file=${secretDir}/role_bindings.yaml --from-file=${secretDir}/opa_authorization.rego
  echo "created rbac secret opt-data"

  export REGISTRY=quay.io/open-cluster-management-hub-of-hubs
  export IMAGE_TAG="$TAG"
  export COMPONENT=multicluster-globalhub-rbac
  envsubst < ${currentDir}/hoh/multicluster-globalhub-rbac/operator.yaml | kubectl apply -f - -n "$namespace"
  kubectl wait deployment -n "$namespace" "$COMPONENT" --for condition=Available=True --timeout=600s
  kubectl scale deployment "$COMPONENT" -n "$namespace" --replicas=1
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
  kubectl patch clustermanager cluster-manager --type merge -p '{"spec":{"placementImagePullSpec":"quay.io/open-cluster-management-hub-of-hubs/placement:hub-of-hubs@sha256:b7293b436dc00506b370762fb4eb352e7c6cc5413d135fc03c93ed311e7ed4c4"}}'
 
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