#!/bin/bash

export KUBECONFIG=${KUBECONFIG:-$1}
echo "KUBECONFIG=$KUBECONFIG"

pgha="$(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master |grep postgres-pgha |awk '{print $1}' || true)"
if [ "$pgha" != "" ]; then
    echo "database pod $pgha"
fi 

sql="SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, compliance as status, COUNT(1) as count
FROM local_status.compliance
GROUP BY compliance;"

while [ true ]; do
    sleep 60
    # kubectl exec -it $pgha -c database -n multicluster-global-hub -- psql -U postgres -d hoh -c "$sql"
    kubectl exec -t $pgha -c database -n multicluster-global-hub -- psql -U postgres -d hoh -t -c "$sql"
done
