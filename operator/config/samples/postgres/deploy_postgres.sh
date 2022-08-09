#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# install postgres operator
POSTGRES_OPERATOR=${POSTGRES_OPERATOR:-"pgo"}
kubectl apply -k ${currentDir}/postgres-operator
kubectl -n postgres-operator wait --for=condition=Available Deployment/$POSTGRES_OPERATOR --timeout=1000s
echo "$POSTGRES_OPERATOR is ready!"

# deploy postgres cluster
pgnamespace="hoh-postgres"
userSecret="hoh-pguser-postgres"

kubectl apply -k ${currentDir}/postgres-cluster
matched=$(kubectl get secret $userSecret -n $pgnamespace --ignore-not-found=true)
SECOND=0
while [ -z "$matched" ]; do
  if [ $SECOND -gt 300 ]; then
    echo "Timeout waiting for creating $secret"
    exit 1
  fi
  echo "Waiting for secret $userSecret to be created in pgnamespace $pgnamespace"
  matched=$(kubectl get secret $userSecret -n $pgnamespace --ignore-not-found=true)
  sleep 5
  (( SECOND = SECOND + 5 ))
done

# patch the postgres stateful
stss=$(kubectl get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done
# delete all pods to recreate in case the pod won't be restarted when the statefulset is patched
kubectl delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null
echo "Postgres is ready!"

# create usersecret for postgres
databaseHost="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "host" | base64decode}}')"
databasePort="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "port" | base64decode}}')"
databaseUser="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "user" | base64decode}}')"
databasePassword="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "password" | base64decode}}')"
databasePassword=$(printf %s "$pgAdminPassword" |jq -sRr @uri)

databaseUri="postgres://${databaseUser}:${databasePassword}@${databaseHost}:${pgAdminPort}/hoh"

kubectl create secret generic postgresql-secret -n "open-cluster-management" \
    --from-literal=database_uri=$databaseUri
echo "Postgresql-secret is ready!"