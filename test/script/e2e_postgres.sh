#!/bin/bash

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

KUBECONFIG=${1:-$KUBECONFIG}

start_time=$(date +%s)
echo -e "\r${BOLD_GREEN}[ START ] Install Postgres $NC"

# step1: check storage secret
target_namespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
storage_secret=${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"}
if kubectl get secret "$storage_secret" -n "$target_namespace"; then
  echo "storage_secret $storage_secret already exists in $target_namespace namespace"
  exit 0
fi

# installed credentials
pg_ns="hoh-postgres"
ps_user="hoh-pguser-postgres"
pg_cert="hoh-cluster-cert"

# step2: deploy postgres operator pgo
retry "kubectl apply --server-side -k $TEST_DIR/manifest/postgres/postgres-operator && (kubectl get pods -n postgres-operator | grep pgo | grep Running)" 60

# step3: deploy  postgres cluster
retry "kubectl apply -k $TEST_DIR/manifest/postgres/postgres-cluster && (kubectl get secret $ps_user -n $pg_ns)"

# expose the postgres service as NodePort
kubectl patch postgrescluster hoh -n $pg_ns -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}'  --type merge

stss=$(kubectl get statefulset -n $pg_ns -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset "$sts" -n $pg_ns -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done

kubectl delete pod -n $pg_ns --all --ignore-not-found=true 2>/dev/null  

# postgres
wait_cmd "kubectl get pods -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pg_ns | grep Running"
kubectl wait --for=condition=ready pod -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pg_ns--timeout=100s
echo "Postgres cluster is ready!"

database_uri=$(kubectl get secrets -n "${pg_ns}" "${ps_user}" -o go-template='{{index (.data) "uri" | base64decode}}')
kubectl get secret $pg_cert -n $pg_ns -o jsonpath='{.data.ca\.crt}' |base64 -d > "$CONFIG_DIR/postgres-cluster-ca.crt"

# covert the database uri into external uri
external_host=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed -e 's#^https\?://##' -e 's/:.*//')
external_port=32432
database_uri=$(echo "${database_uri}" | sed "s|@[^/]*|@$external_host:$external_port|")

# step4: generate storage secret
kubectl create namespace "$target_namespace" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic "$storage_secret" -n "$target_namespace" \
    --from-literal=database_uri="${database_uri}?sslmode=verify-ca" \
    --from-file=ca.crt="$CONFIG_DIR/postgres-cluster-ca.crt"

echo "Storage secret is ready in $target_namespace namespace!"

echo -e "\r${BOLD_GREEN}[ END - $(date +"%T") ] Install Postgres ${NC} $(($(date +%s) - start_time)) seconds"
