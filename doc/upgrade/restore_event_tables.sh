#!/bin/bash

# Tables: event.local_policies , event.local_root_policies, and event.managed_clusters

# Set namespace and pod names
NAMESPACE="${1:-multicluster-global-hub}"
SOURCE_POD="multicluster-global-hub-postgres-0"       # Source pod
SOURCE_CONTAINER="multicluster-global-hub-postgres"   # Source container
TARGET_POD="multicluster-global-hub-postgresql-0"     # Target pod
TARGET_CONTAINER="multicluster-global-hub-postgresql" # Target container
REMOTE_PATH="/tmp/event_tables_backup.sql"            # Without leading `/` to suppress tar warnings
DB_NAME="hoh"
DB_USER="postgres"

echo "Using namespace: $NAMESPACE"

# Backup from source pod
echo ">> Backing up event tables from $SOURCE_POD ($SOURCE_CONTAINER)..."
kubectl exec -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- pg_dump -U $DB_USER -d $DB_NAME -t 'event.*' --data-only -f $REMOTE_PATH 2>/dev/null
kubectl cp $NAMESPACE/$SOURCE_POD:$REMOTE_PATH ./event_tables_backup.sql -c $SOURCE_CONTAINER >/dev/null 2>&1
echo ">> Backup completed."

# List of tables to process
tables=("event.local_policies" "event.local_root_policies" "event.managed_clusters")

echo ">> Removing duplicated event records from the target tables"

for table in "${tables[@]}"; do
  echo "> Processing table: $table"

  # Get the latest timestamp from the source table
  latest_time=$(kubectl exec -it -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -t -c "SELECT MAX(created_at) FROM $table;")

  latest_time=$(echo $latest_time | xargs) # Trim whitespace

  # Validate if latest_time is empty, NULL, or not a valid timestamp
  if [ -z "$latest_time" ] || [ "$latest_time" == "NULL" ] || ! [[ "$latest_time" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2} ]]; then
    echo "  No records(or invalid) found in $table. Skipping..."
    continue
  fi

  echo "> Latest created_at time for $table: $latest_time"
  # Delete records from the target table with a timestamp earlier than the latest time
  echo "> Deleting records from $table with created_at <= $latest_time"
  kubectl exec -it -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -c "DELETE FROM $table WHERE created_at <= '$latest_time';"
done

echo ">> Completed removing duplicated event records."

# Restore to target pod
echo ">> Restoring event tables to $TARGET_POD ($TARGET_CONTAINER)..."
kubectl cp ./event_tables_backup.sql $NAMESPACE/$TARGET_POD:$REMOTE_PATH -c $TARGET_CONTAINER
kubectl exec -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- psql -U $DB_USER -d $DB_NAME -f $REMOTE_PATH
echo ">> Restore completed."

# Cleanup
echo ">> Cleaning up temporary files..."
kubectl exec -c $SOURCE_CONTAINER $SOURCE_POD -n $NAMESPACE -- rm $REMOTE_PATH
kubectl exec -c $TARGET_CONTAINER $TARGET_POD -n $NAMESPACE -- rm $REMOTE_PATH
rm ./event_tables_backup.sql
echo ">> Local and remote backup files removed. Done!"
