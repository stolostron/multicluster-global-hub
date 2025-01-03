#!/bin/bash

# Tables: `history.local_compliance` and `history.local_compliance_job_log`

# Set namespace and pod names
NAMESPACE="${1:-multicluster-global-hub}"
SOURCE_POD="multicluster-global-hub-postgres-0"       # Source pod
SOURCE_CONTAINER="multicluster-global-hub-postgres"   # Source container
TARGET_POD="multicluster-global-hub-postgresql-0"     # Target pod
TARGET_CONTAINER="multicluster-global-hub-postgresql" # Target container
REMOTE_PATH="/tmp/history_tables_backup.sql"          # Without leading `/` to suppress tar warnings
DB_NAME="hoh"
DB_USER="postgres"

echo "Using namespace: $NAMESPACE"

# Backup from source pod
echo ">> Backing up history tables from $SOURCE_POD ($SOURCE_CONTAINER)..."
kubectl exec -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- pg_dump -U $DB_USER -d $DB_NAME -t 'history.*' --data-only -f $REMOTE_PATH 2>/dev/null
kubectl cp $NAMESPACE/$SOURCE_POD:$REMOTE_PATH ./history_tables_backup.sql -c $SOURCE_CONTAINER >/dev/null 2>&1
echo ">> Backup completed."

echo ">> Removing duplicated history records from the target tables"

table="history.local_compliance"
echo "> Processing table: $table"

# Get the latest timestamp from the source table
latest_time=$(kubectl exec -it -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -t -c "SELECT MAX(compliance_date) FROM $table;")
latest_time=$(echo $latest_time | xargs) # Trim whitespace

# Validate if latest_time is empty, NULL, or not a valid timestamp
if [ -z "$latest_time" ] || [ "$latest_time" == "NULL" ] || ! [[ "$latest_time" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2} ]]; then
  echo ">  No records(or invalid) found in $table"
else
  # Delete records from the target table with a compliance_date earlier than the latest_time
  echo "> Deleting records from $table with compliance_date <= $latest_time"
  kubectl exec -it -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -c "DELETE FROM $table WHERE compliance_date <= '$latest_time';"
fi

echo ">> Completed removing duplicated history records."

# Restore to target pod
echo ">> Restoring history tables to $TARGET_POD ($TARGET_CONTAINER)..."
kubectl cp ./history_tables_backup.sql $NAMESPACE/$TARGET_POD:$REMOTE_PATH -c $TARGET_CONTAINER
kubectl exec -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -f $REMOTE_PATH
echo ">> Restore completed."

# Cleanup
echo ">> Cleaning up temporary files..."
kubectl exec -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- rm $REMOTE_PATH
kubectl exec -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- rm $REMOTE_PATH
rm ./history_tables_backup.sql
echo ">> Local and remote backup files removed. Done!"
