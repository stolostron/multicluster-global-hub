# !/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}

rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." ; pwd -P)"
postgresDir="$rootDir/operator/config/samples/storage"

bash $postgresDir/deploy_postgres.sh $KUBECONFIG

# expose the postgres service as NodePort
kubectl patch postgrescluster hoh -n $pgnamespace -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}'  --type merge

pgnamespace="hoh-postgres"
stss=$(kubectl get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null  
echo "Postgres is pathed!"



# step4: generate storage secret
pgnamespace="hoh-postgres"

userSecret="hoh-pguser-postgres"
databaseHost="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "host" | base64decode}}')"
databasePort="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "port" | base64decode}}')"
databaseUser="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "user" | base64decode}}')"
databasePassword="$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "password" | base64decode}}')"
databasePassword=$(printf %s "$databasePassword" |jq -sRr @uri)

kubectl create secret generic $storageSecret -n $targetNamespace \
    --from-literal=database_uri="postgres://${databaseUser}:${databasePassword}@${databaseHost}:${pgAdminPort}/hoh"
echo "storage secret is ready in $targetNamespace namespace!"