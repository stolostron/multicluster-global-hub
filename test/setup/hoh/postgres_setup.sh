# !/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}

rootDir="$(cd "$(dirname "$0")/../../.." ; pwd -P)"
postgresDir="$rootDir/operator/config/samples/storage"

bash $postgresDir/deploy_postgres.sh $KUBECONFIG

pgnamespace="hoh-postgres"
stss=$(kubectl get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null  
echo "Postgres is pathed!"

