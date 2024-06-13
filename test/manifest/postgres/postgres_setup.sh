#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}

current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
test_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." || exit ; pwd -P)"
# shellcheck source=/dev/null
source "$test_dir/script/util.sh"

start_time=$(date +%s)
echo -e "\r${BOLD_GREEN}[ START ] Install Postgres $NC"

# step1: check storage secret
target_namespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
storage_secret=${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"}
if [ -n "$(kubectl get secret "$storage_secret" -n "$target_namespace" --ignore-not-found=true)" ]; then
  echo "storage_secret $storage_secret already exists in $TARGET_NAMESPACE namespace"
  exit 0
fi

# step2: deploy postgres operator pgo
retry "kubectl apply --server-side -k $current_dir/postgres-operator && (kubectl get pods -n postgres-operator | grep pgo | grep Running)" 60

# step3: deploy  postgres cluster
retry "kubectl apply -k ${current_dir}/postgres-cluster && (kubectl get secret hoh-pguser-postgres -n hoh-postgres)"

# step4: generate storage secret 
pgnamespace="hoh-postgres"
userSecret="hoh-pguser-postgres"
certSecret="hoh-cluster-cert"

databaseURI=$(kubectl get secrets -n "${pgnamespace}" "${userSecret}" -o go-template='{{index (.data) "uri" | base64decode}}')
kubectl get secret $certSecret -n $pgnamespace -o jsonpath='{.data.ca\.crt}' |base64 -d > "$current_dir/ca.crt"

kubectl create namespace "$target_namespace" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic "$storage_secret" -n "$target_namespace" \
    --from-literal=database_uri="${databaseURI}?sslmode=verify-ca" \
    --from-file=ca.crt="$current_dir/ca.crt "

echo "storage secret is ready in $target_namespace namespace!"

# expose the postgres service as NodePort
kubectl patch postgrescluster hoh -n $pgnamespace -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}'  --type merge

stss=$(kubectl get statefulset -n $pgnamespace -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset "$sts" -n $pgnamespace -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl delete pod -n $pgnamespace --all --ignore-not-found=true 2>/dev/null  
echo "Postgres is pathed!"

# postgres
wait_cmd "kubectl get pods -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pgnamespace | grep Running"
kubectl wait --for=condition=ready pod -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pgnamespace--timeout=100s

echo -e "\r${BOLD_GREEN}[ END ] Install Postgres ${NC} $(($(date +%s) - start_time)) seconds"