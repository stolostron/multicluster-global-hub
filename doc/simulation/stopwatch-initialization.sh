#!/bin/bash

export KUBECONFIG=${KUBECONFIG:-$1}
echo "KUBECONFIG=$KUBECONFIG"

pgha="$(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master |grep postgres-pgha |awk '{print $1}' || true)"
if [ "$pgha" != "" ]; then
    echo "database pod: $pgha"
fi 

sql="SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, table_name, COUNT(1) AS count
FROM (
    SELECT 'cluster' AS table_name FROM status.managed_clusters
    UNION ALL
    SELECT 'compliance' AS table_name FROM local_status.compliance
    UNION ALL
    SELECT 'event' AS table_name FROM "event".local_policies
) AS subquery
GROUP BY table_name"

# sql="SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS'), COUNT(1) FROM status.managed_clusters"

while [ true ]; do
    sleep 0.4
    # kubectl exec -it $pgha -c database -n multicluster-global-hub-postgres -- psql -U postgres -d hoh -c "$sql"
    kubectl exec -t $pgha -c database -n multicluster-global-hub -- psql -U postgres -d hoh -t -c "$sql"
done