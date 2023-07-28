# !/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}

rootDir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." ; pwd -P)"
postgresDir="$rootDir/operator/config/samples/storage"

bash $postgresDir/deploy_postgres.sh $KUBECONFIG

# expose the postgres service as NodePort
pgnamespace="hoh-postgres"
kubectl --kubeconfig $KUBECONFIG patch postgrescluster hoh -n $pgnamespace -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}'  --type merge
# unset HA
kubectl --kubeconfig $KUBECONFIG patch configmap pgo-config -n hoh-postgres --type json -p='[{"op": "replace", "path": "/data/DisableAutofail", "value": "true"}]'
kubectl --kubeconfig $KUBECONFIG patch postgrescluster hoh -n hoh-postgres --type json -p='[{"op": "replace", "path": "/spec/instances/0/replicas", "value": 1}]'
kubectl --kubeconfig $KUBECONFIG patch postgrescluster hoh -n hoh-postgres --type merge --patch '{"spec":{"proxy":{"pgBouncer":{"replicas":1}}}}'

stss=$(kubectl --kubeconfig $KUBECONFIG get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl --kubeconfig $KUBECONFIG patch statefulset ${sts} -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl --kubeconfig $KUBECONFIG delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null  
echo "Postgres is pathed!"
