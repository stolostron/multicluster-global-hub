# Built-in PostgreSQL Configuration

## Initialize PostgreSQL User and Database via Global Hub API Annotation

To enhance the functionality of the Global Hub as a resource inventory, we provide an API annotation for Global Hub admins to easily initialize users and databases. Typically, users are granted read and write permissions only for the databases they create. The annotation key is `global-hub.open-cluster-management.io/postgres-users`, and its value is a JSON list that allows initializing multiple users at once. The format is:

```json
[{"name": "<user>", "databases": ["<db1>", "<db2>"]}]
```

#### Example:

```bash
$ kubectl patch mgh multiclusterglobalhub -n multicluster-global-hub --type='merge' \
  -p '{"metadata": {"annotations": {"global-hub.open-cluster-management.io/postgres-users": "[{\"name\": \"testuser\", \"databases\": [\"test1\"]}]"}}}'
$ kubectl get mgh multiclusterglobalhub -n multicluster-global-hub -o yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    global-hub.open-cluster-management.io/postgres-users: '[{"name": "testuser", "databases": ["test1", "test2"]}]'
  ...
```

After the annotation is applied, the control plane will generate a secret named `postgresql-user-<username>`. In the example above, the secret name will be `postgresql-user-testuser`. The secret will contain credentials for accessing the database by the user you specified. The details will appear as follows:

```bash
apiVersion: v1
data:
  ca: LS...K
  databases: dGVzdDEsdGVzdDI=
  db.host: ***
  db.password: ***
  db.port: NTQzMg==
  db.user: dGVzdHVzZXI=
kind: Secret
metadata:
  creationTimestamp: "2025-01-10T09:55:32Z"
  name: postgresql-user-testuser
  namespace: multicluster-global-hub
...
```

## Custom PostgreSQL Configuration

The Global Hub also provides a way to customize the configuration of the built-in PostgreSQL server (refer to [PostgreSQL Configuration Settings](https://www.postgresql.org/docs/16/config-setting.html#CONFIG-SETTING-CONFIGURATION-FILE)). Follow these steps to achieve it:

1. Create a secret named `multicluster-global-hub-custom-postgresql-config` containing the `postgresql.conf`, as shown below:

```yaml
apiVersion: v1
data:
  postgresql.conf: |
    wal_level = logical
    max_wal_size = 2GB
kind: ConfigMap
metadata:
  ...
  creationTimestamp: "2025-01-13T08:40:51Z"
  name: multicluster-global-hub-custom-postgresql-config
  namespace: multicluster-global-hub
```

2. Restart the PostgreSQL server instance to apply the configuration changes:

```
kubectl delete pods -l name=multicluster-global-hub-postgresql
```