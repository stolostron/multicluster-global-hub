# Builtin Postgresql Configuration

This section only introduce the features supported by builtin and postgres configuration provide by the global hub control plane

## Initialize the Postgres user and database by Annotation of Global Hub API

To enrich the functionality of the global hub as an resource inventory. We will expose the interface by the annotation of the API to make sure the global hub admin can initialize their user and database conveniently. Typically the user can only have the permission(write and read) for the created database. The anntation key is `global-hub.open-cluster-management.io/postgres-users` and the value is a json list which means you can initialized multi users at once, the format is `[{"name": "<user>", "databases": ["<db1>", "<db2>"]}]`. The following is an example

```bash
$ kubectl patch mgh multiclusterglobalhub -n multicluster-global-hub --type='merge' \
  -p '{"metadata": {"annotations": {"global-hub.open-cluster-management.io/postgres-users": "[{\"name\": \"testuser\", \"databases\": [\"test1\"]}]"}}}'
$ kubectl get mgh multiclusterglobalhub -n multicluster-global-hub -oyaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    global-hub.open-cluster-management.io/postgres-users: '[{"name": "testuser", "databases":
      ["test1", "test2"]}]'
  ...
```

After the annotation is patched, the control-plane will generate the secret with name `postgresql-user-<username>`, for the above example is `postgresql-user-testuser `. The secret will contains the credential on how to access the database by the user you just patched, the detial as follows

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


## Postgres Customized Configuration

The global hub also provide a way to customized the configuration(https://www.postgresql.org/docs/16/config-setting.html#CONFIG-SETTING-CONFIGURATION-FILE) for the the builtin postgres sever, you can following the following step to achieve it.

1. Create the secret named `multicluster-global-hub-custom-postgresql-config`, which contains the configuration `postgresql.conf`. As the following:

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

2. Restart the instance of the postgres server to ensure the configuration take effect.

```
kubectl delete pods -l name=multicluster-global-hub-postgresql
```