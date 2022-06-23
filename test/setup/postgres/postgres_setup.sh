#!/bin/bash
# Source this script to enable the postgres in hub-of-hubs. DATABASE_URL_HOH and DATABASE_URL_HOH could be used to init hub-of-hubs

function installPostgres() {
  currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  namespace="hoh-postgres"
  userSecrets=("hoh-pguser-hoh-process-user" "hoh-pguser-postgres" "hoh-pguser-transport-bridge-user")

  # ensure the pgo operator crd and other stuff is deleted first to start its deployment from scratch
  kubectl delete -k ${currentDir}/install --ignore-not-found=true 2>/dev/null

  # ensure all the user secrets are deleted
  for secret in ${userSecrets[@]}
  do
    matched=$(kubectl get secret $secret -n $namespace --ignore-not-found=true)
    while [ ! -z "$matched" ]; do
      echo "Waiting for secret $secret to be deleted from namespace $namespace"
      matched=$(kubectl get secret $secret -n $namespace --ignore-not-found=true)
      sleep 10
    done
  done

  # install the pgo operator to postgres-operator
  kubectl apply -k ${currentDir}/install

  # ensure all the user secrets are created
  for secret in ${userSecrets[@]}
  do
    matched=$(kubectl get secret $secret -n $namespace --ignore-not-found=true)
    while [ -z "$matched" ]; do
      echo "Waiting for secret $secret to be created in namespace $namespace"
      matched=$(kubectl get secret $secret -n $namespace --ignore-not-found=true)
      sleep 10
    done
  done

  kubectl delete -f ${currentDir}/postgres-job.yaml --ignore-not-found=true

  IMAGE=quay.io/open-cluster-management-hub-of-hubs/postgresql-ansible:$TAG 
  envsubst < ${currentDir}/postgres-job.yaml | kubectl apply -f -

  kubectl wait --for=condition=complete job/postgres-init -n $namespace --timeout=300s
  kubectl logs $(kubectl get pods --field-selector status.phase=Succeeded  --selector=job-name=postgres-init -n $namespace  --output=jsonpath='{.items[*].metadata.name}') -n $namespace
}

# always check whether DATABASE_URL_HOH and DATABASE_URL_TRANSPORT are set, if not - install PGO and use its secrets
if [ -z "${DATABASE_URL_HOH-}" ] && [ -z "${DATABASE_URL_TRANSPORT-}" ]; then
  installPostgres

  namespace="hoh-postgres"
  processUser="hoh-pguser-hoh-process-user"
  transportUser="hoh-pguser-transport-bridge-user"

  DATABASE_URL_HOH="$(kubectl get secrets -n "${namespace}" "${processUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
  DATABASE_URL_TRANSPORT="$(kubectl get secrets -n "${namespace}" "${transportUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
fi