# Built-in PostgreSQL Configuration

## Initialize PostgreSQL User and Database

To enhance the functionality of the Global Hub as a resource inventory, we utilize a ConfigMap to allow Global Hub admins to easily initialize users and databases. Typically, users are granted read and write permissions only for the databases they create. You can create the ConfigMap named `multicluster-global-hub-custom-postgresql-users`, and the data format as follows:

```yaml
<userName>: "[<database1>, <database2>]"
```

#### Example:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: multicluster-global-hub-custom-postgresql-users
  namespace: multicluster-global-hub
data:
  "testuser": '["test1", "test2"]'
EOF
```

After the configMap is applied, the control plane will generate a secret named `postgresql-user-<username>`. In the example above, the secret name will be `postgresql-user-testuser`. The secret will contain credentials for accessing the database by the user you specified. The details will appear as follows:

```bash
apiVersion: v1
data:
  db.ca_cert: LS...K
  db.names: WyJ0ZXN0MSIsICJ0ZXN0MiJd
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

Note: If the `<username>` contains characters '_', they will be replaced with '-' in the secret name. For example, if the `<username>` is `test_user`, the secret name will be `postgresql-user-test-user`.

## Custom PostgreSQL Server Configuration

The Global Hub also provides a way to customize the configuration of the built-in PostgreSQL server (refer to [PostgreSQL Configuration Settings](https://www.postgresql.org/docs/16/config-setting.html#CONFIG-SETTING-CONFIGURATION-FILE)). Follow these steps to achieve it:

1. Create a ConfigMap named `multicluster-global-hub-custom-postgresql-config` containing the `postgresql.conf`, as shown below:

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