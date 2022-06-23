#!/bin/bash
# Source this script to enable the postgres in hub-of-hubs. DATABASE_URL_HOH and DATABASE_URL_HOH could be used to init hub-of-hubs

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

function enablePostgresOperator() {
  POSTGRES_OPERATOR=${POSTGRES_OPERATOR:-"pgo"}
  # ensure the pgo operator crd and other stuff is deleted first to start its deployment from scratch
  kubectl delete -k ${currentDir}/postgres-operator --ignore-not-found=true 2>/dev/null
  # install the pgo operator to postgres-operator
  kubectl apply -k ${currentDir}/postgres-operator
  kubectl -n postgres-operator wait --for=condition=Available Deployment/$POSTGRES_OPERATOR --timeout=1000s
  echo "$POSTGRES_OPERATOR is ready!"
}

function deployPostgresCluster() {
  namespace="hoh-postgres"
  userSecrets=("hoh-pguser-hoh-process-user" "hoh-pguser-postgres" "hoh-pguser-transport-bridge-user")

  # ensure the pgo operator is deleted first to start its deployment from scratch
  kubectl delete -k ${currentDir}/postgres-cluster --ignore-not-found=true 2>/dev/null

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

  kubectl apply -k ${currentDir}/postgres-cluster

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
}


# always check whether DATABASE_URL_HOH and DATABASE_URL_TRANSPORT are set, if not - install PGO and use its secrets
if [ -z "${DATABASE_URL_HOH-}" ] && [ -z "${DATABASE_URL_TRANSPORT-}" ]; then
  enablePostgresOperator
  deployPostgresCluster

  namespace="hoh-postgres"
  processUser="hoh-pguser-hoh-process-user"
  transportUser="hoh-pguser-transport-bridge-user"

  DATABASE_URL_HOH="$(kubectl get secrets -n "${namespace}" "${processUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
  DATABASE_URL_TRANSPORT="$(kubectl get secrets -n "${namespace}" "${transportUser}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
fi