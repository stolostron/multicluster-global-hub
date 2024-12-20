# Upgrade the Built-in Postgres from Global Hub Release-1.3.0 to 1.4.0

With the Global Hub Release 1.4.0, the built-in Postgres database has been upgraded to version 16, replacing the existing statefulset instance `multicluster-global-hub-postgres` (version 13) with a new instance, `multicluster-global-hub-postgresql`. By default, after the upgrade, real-time data such as **policies** and **clusters** will be automatically re-synced to the new Postgres instance.

However, historical data (e.g., **event** and **history** tables) will not be recovered automatically. If retaining historical data is not a concern, you can skip the backup and restore steps below. Otherwise, follow the optional backup and restore instructions.

## Backup and Restore Data(history and event) [Optional]

- Run the following script to restore history tables

```bash
./doc/upgrade/restore_history_tables.sh
```

- Run the following script to restore event tables

```bash
./doc/upgrade/restore_event_tables.sh
```

## Delete Legacy Built-in Postgres Resources Manually [Optional]

After the upgrade, the Global Hub switches to the new built-in Postgres instance. Resources associated with the legacy Postgres instance are not deleted automatically by the Global Hub operator. To clean up these resources, Use the following script to delete legacy Postgres resources.

 ```bash
 ./doc/upgrade/cleanup_legacy_postgres.sh
 ```