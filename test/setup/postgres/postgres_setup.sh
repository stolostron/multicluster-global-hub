#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
setupDir="$(cd "$(dirname "$0")/.." ; pwd -P)"
pgIsReady=${setupDir}/config/postgres_is_ready

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
  pgnamespace="hoh-postgres"
  userSecrets=("hoh-pguser-hoh-process-user" "hoh-pguser-postgres" "hoh-pguser-transport-bridge-user")

  # ensure the pgo operator is deleted first to start its deployment from scratch
  kubectl delete -k ${currentDir}/postgres-cluster --ignore-not-found=true

  # ensure all the user secrets are deleted
  for secret in ${userSecrets[@]}
  do
    matched=$(kubectl get secret $secret -n $pgnamespace --ignore-not-found=true)
    SECOND=0
    while [ ! -z "$matched" ]; do
      if [ $SECOND -gt 300 ]; then
        echo "Timeout waiting for deleting $secret"
        exit 1
      fi
      echo "Waiting for secret $secret to be deleted from pgnamespace $pgnamespace"
      matched=$(kubectl get secret $secret -n $pgnamespace --ignore-not-found=true)
      sleep 5
      (( SECOND = SECOND + 5 ))
    done
  done

  kubectl apply -k ${currentDir}/postgres-cluster

  # ensure all the user secrets are created
  for secret in ${userSecrets[@]}
  do
    matched=$(kubectl get secret $secret -n $pgnamespace --ignore-not-found=true)
    SECOND=0
    while [ -z "$matched" ]; do
      if [ $SECOND -gt 300 ]; then
        echo "Timeout waiting for creating $secret"
        exit 1
      fi
      echo "Waiting for secret $secret to be created in pgnamespace $pgnamespace"
      matched=$(kubectl get secret $secret -n $pgnamespace --ignore-not-found=true)
      sleep 5
      (( SECOND = SECOND + 5 ))
    done
  done

  # patch the postgres stateful
  stss=$(kubectl get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
  for sts in ${stss}; do
    kubectl patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
  done

  # delete all pods to recreate in case the pod won't be restarted when the statefulset is patched
  kubectl delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null
  
  touch $pgIsReady
  echo "Postgres is ready!"
}

if [[ -f $pgIsReady ]]; then
  echo "Postgres is already ready"
else
  enablePostgresOperator
  deployPostgresCluster
fi