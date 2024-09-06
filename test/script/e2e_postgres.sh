#!/bin/bash

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

POSTGRES_KUBECONFIG=${1:-$KUBECONFIG}
SECRET_KUBECONFIG=${2:-$KUBECONFIG}

echo "POSTGRES_KUBECONFIG=$POSTGRES_KUBECONFIG"
echo "SECRET_KUBECONFIG=$SECRET_KUBECONFIG"

start_time=$(date +%s)
echo -e "\r${BOLD_GREEN}[ START - $(date +"%T") ] Install Postgres $NC"

# step1: check storage secret
target_namespace=${TARGET_NAMESPACE:-"multicluster-global-hub"}
storage_secret=${STORAGE_SECRET_NAME:-"multicluster-global-hub-storage"}
if kubectl get secret "$storage_secret" -n "$target_namespace" --kubeconfig "$SECRET_KUBECONFIG"; then
  echo "storage_secret $storage_secret already exists in $target_namespace namespace"
  exit 0
fi

# installed credentials
pg_ns="hoh-postgres"
ps_user="hoh-pguser-postgres"
# pg_cert="hoh-cluster-cert"

# step2: deploy postgres operator pgo
retry "kubectl apply --server-side --kubeconfig $POSTGRES_KUBECONFIG -k $TEST_DIR/manifest/postgres/postgres-operator && (kubectl get pods -n postgres-operator --kubeconfig $POSTGRES_KUBECONFIG | grep pgo | grep Running)" 60

# step3: deploy  postgres cluster
retry "kubectl apply -k $TEST_DIR/manifest/postgres/postgres-cluster --kubeconfig $POSTGRES_KUBECONFIG && (kubectl get secret $ps_user -n $pg_ns --kubeconfig $POSTGRES_KUBECONFIG)"

# expose the postgres service as NodePort
kubectl patch postgrescluster hoh -n $pg_ns --kubeconfig "$POSTGRES_KUBECONFIG" -p '{"spec":{"service":{"type":"NodePort", "nodePort": 32432}}}' --type merge

stss=$(kubectl get statefulset -n $pg_ns --kubeconfig "$POSTGRES_KUBECONFIG" -o jsonpath={.items..metadata.name})
for sts in ${stss}; do
  kubectl patch statefulset "$sts" -n $pg_ns --kubeconfig "$POSTGRES_KUBECONFIG" -p '{"spec":{"template":{"spec":{"securityContext":{"fsGroup":26}}}}}'
done
kubectl delete pod -n $pg_ns --kubeconfig "$POSTGRES_KUBECONFIG" --all --ignore-not-found=true 2>/dev/null

# postgres
wait_cmd "kubectl get pods --kubeconfig $POSTGRES_KUBECONFIG -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pg_ns | grep Running"
kubectl wait --for=condition=ready pod -l postgres-operator.crunchydata.com/instance-set=pgha1 -n $pg_ns--timeout=100s --kubeconfig "$POSTGRES_KUBECONFIG"
echo "Postgres cluster is ready!"

echo -e "\r${BOLD_GREEN}[ END - $(date +"%T") ] Install Postgres ${NC} $(($(date +%s) - start_time)) seconds"
