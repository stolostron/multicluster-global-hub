# Global Hub backup and restore

This is the procedure for backing up a primary Global Hub and restoring it onto a passive hub. It covers the ACM Velero schedules and the VolSync restic copy of the built-in Postgres volume, plus the failures found while running that path on an LVMS/TopoLVM hub.

The VolSync objects are created by the community policies in [stolostron/policy-collection `acm-hub-pvc-backup`](https://github.com/stolostron/policy-collection/tree/main/community/CM-Configuration-Management/acm-hub-pvc-backup). The template changes those policies still need are tracked in [ACM-48075](https://redhat.atlassian.net/browse/ACM-48075).

Two copies of data move independently:

- ACM `BackupSchedule` writes credentials, resources, and generic resources through Velero. The `BackupStorageLocation` uses the bucket prefix reserved for Velero, for example `velero/hub-<hub-name>`.
- VolSync writes the Postgres PVC to a different prefix in the same bucket, for example `s3:<s3Url>/<bucket>/volsync`. The source policy appends `/<namespace>-<pvc-name>` for each volume. Do not point `RESTIC_REPOSITORY` at the Velero prefix.

The built-in Postgres claim is `postgresdb-multicluster-global-hub-postgresql-0` in `multicluster-global-hub`. It has to carry the label `cluster.open-cluster-management.io/backup-hub-pvc`. The label value can be any string.

## Policies

Install the policy set on the primary hub and on the passive hub. Placement matches `local-cluster` or a managed cluster with `is-hub=true`.

| Policy | What it does |
| --- | --- |
| `acm-hub-pvc-backup-config` | Installs the VolSync addon. `check-hub-storage-configmap` is `inform`. It becomes Compliant when ConfigMap `hub-pvc-backup` and Secret `acm-hub-pvc-backup-restic-secret` exist in `open-cluster-management-backup` with the backup label the policy expects. It does not read the data keys. An empty Secret satisfies it. |
| `acm-hub-pvc-backup-source` | `enforce`, and it does not run until the config policy is Compliant. For each Bound PVC with the hub-pvc label it creates a per-PVC ConfigMap, a per-PVC restic Secret, a `ReplicationSource`, and ConfigMap `hub-pvc-backup-pvcs`. |
| `acm-hub-pvc-backup-destination` | On the passive hub, after the credentials backup restores `hub-pvc-backup-pvcs`, it creates a `ReplicationDestination` for each listed PVC. |

The community policy looks for label `cluster.open-cluster-management.io/backup: cluster-activation` on the ConfigMap and Secret. The objects you create must use the value that the copy of the policy installed on the hub actually checks.

## Objects you create

Create these on the primary hub, in `open-cluster-management-backup`, before expecting a `ReplicationSource`.

### ConfigMap `hub-pvc-backup`

`trigger_schedule` is required. An empty value is copied into `spec.trigger.schedule`, and the `ReplicationSource` is rejected with `Invalid value: ""`.

For an LVMS hub the snapshot class has to be the one that can snapshot the Postgres volume. `copyMethod` defaults to `Snapshot` when the key is omitted.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hub-pvc-backup
  namespace: open-cluster-management-backup
  labels:
    cluster.open-cluster-management.io/backup: cluster-activation
data:
  pruneIntervalDays: "14"
  retain_last: "5"
  storageClassName: lvms-assisted-nvme
  volumeSnapshotClassName: lvms-assisted-nvme
  trigger_schedule: "0 */6 * * *"
```

A ConfigMap named `hub-pvc-backup-<namespace>-<pvc-name>` in the same namespace overrides these values for one PVC.

### Secret `acm-hub-pvc-backup-restic-secret`

The source template reads `data.RESTIC_REPOSITORY`, runs `base64dec`, appends `/<namespace>-<pvc-name>`, and base64-encodes that string into the per-PVC Secret. Every other key is copied through. If `RESTIC_REPOSITORY` is missing, the whole template aborts:

```text
executing "tmpl" at <base64dec>: invalid value; expected string
```

The config policy can already be Compliant when that happens, because it only checks that the Secret exists. Fill the keys before relying on the source policy. Take the values from the hub's existing backup location. Do not invent a second set of credentials.

| Key | Source |
| --- | --- |
| `RESTIC_REPOSITORY` | `s3:<BackupStorageLocation spec.config.s3Url>/<bucket>/volsync` |
| `RESTIC_PASSWORD` | Secret `velero-repo-credentials`, key `repository-password` |
| `AWS_ACCESS_KEY_ID` | Secret `cloud-credentials`, key `cloud`, INI field `aws_access_key_id` under `[default]` |
| `AWS_SECRET_ACCESS_KEY` | The same INI file, field `aws_secret_access_key` |
| `AWS_DEFAULT_REGION` | `BackupStorageLocation` region. For the lab MinIO location that value is `minio`. |

The per-PVC Secret is named `<pvc-name>-acm-pvc-restic` in the PVC namespace. For the built-in database its repository path ends in `/multicluster-global-hub-postgresdb-multicluster-global-hub-postgresql-0`.

ConfigMap `hub-pvc-backup-pvcs` is created by the source policy. Its `data.pvcs` entry looks like `multicluster-global-hub#postgresdb-multicluster-global-hub-postgresql-0`. That ConfigMap is what the destination policy restores from, so it has to be included in the credentials backup.

## Primary hub: confirm the backup

1. `BackupSchedule` phase is `Enabled`, and the `BackupStorageLocation` is `Available`.
2. The Postgres PVC is `Bound` and labeled `cluster.open-cluster-management.io/backup-hub-pvc`.
3. The config policy is Compliant and the source policy has created the `ReplicationSource`.
4. The mover Job runs in `multicluster-global-hub` and the `ReplicationSource` reaches `latestMoverStatus.result: Successful`.

A sync that is already running normally finishes in seconds once the mover can schedule and reach the repository. `WaitingForSchedule` means the cron has not fired again. It does not mean the current sync is still copying.

### Mover Pending on a node-local volume

TopoLVM (`provisioner: topolvm.io`, `volumeBindingMode: WaitForFirstConsumer`) pins the source PV to one node with `topology.topolvm.io/node`. The VolSync snapshot clone PVC is pinned to that same node. The cache PVC is `WaitForFirstConsumer` as well, and it binds on the node where the mover Job is scheduled.

The community template does not set `spec.restic.moverAffinity`. The scheduler can place the mover on a different node from the snapshot. The pod then stays Pending:

```text
0/3 nodes didn't match PersistentVolume node affinity
```

The source template uses `complianceType: mustonlyhave`. A `moverAffinity` added only on the live `ReplicationSource` is removed the next time the policy enforces. The affinity has to be part of the template.

Until [ACM-48075](https://redhat.atlassian.net/browse/ACM-48075) is in the policy you install, patch the root `Policy` `acm-hub-pvc-backup-source` so that, when the source PV has that topology key, the `ReplicationSource` includes:

```yaml
moverAffinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: topology.topolvm.io/node
          operator: In
          values:
          - <node from the source PV>
```

Place that block before `sourcePVC`. Re-applying the stock community policy drops the block.

After the template renders the affinity, delete the stuck mover Job and the cache PVC that bound to the wrong node. Leave the `ReplicationSource` in place. Deleting the `ReplicationSource`, or deleting the Job while you still want that sync, cancels the backup. VolSync then reports `WaitingForSchedule` instead of retrying the sync that was in progress.

### Mover cannot reach the object store

`restic init` fails with `dial tcp <repository>:9000: i/o timeout` when the mover is inside `multicluster-global-hub`. That namespace has NetworkPolicy `default-deny-all`. `allow-dns-and-api` permits DNS and the Kubernetes API only. Velero, running in `open-cluster-management-backup`, can open the same address.

[ACM-47197](https://redhat.atlassian.net/browse/ACM-47197) is fixed in the operator. When the VolSync API is installed, the operator creates NetworkPolicy `volsync-mover`, which allows all egress from pods labeled `app.kubernetes.io/created-by: volsync`. On a hub whose operator does not yet emit that policy, add an egress rule for the repository address and TCP port (MinIO uses 9000) selected by that same label.

## Passive hub: restore

Install ACM and the Global Hub operator on the passive hub. Restore the hub data before the operator creates a new Postgres PVC that races the VolSync destination. See [ACM-47405](https://redhat.atlassian.net/browse/ACM-47405).

Create the ACM restore in `open-cluster-management-backup`:

```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: passive-restore
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroCredentialsBackupName: latest
  veleroManagedClustersBackupName: skip
  veleroResourcesBackupName: latest
```

`cleanupBeforeRestore` has to be `None`. The lowercase value `none` is invalid, and the restore ends `FinishedWithErrors` with `invalid CleanupBeforeRestore value : none`.

ACM creates three Velero restores and waits. The Velero controller runs one restore at a time, so the resources and generic restores stay without a status while the credentials restore is `InProgress`. The ACM `lastMessage` names the Velero restore that is actually executing.

### Which object a stuck credentials restore is on

`itemsRestored` counts objects Velero has already finished. The log line `Restored N` names the last of those. The object it is waiting on shows up in the apiserver audit log for the Velero service account. A restore blocked on a namespace issues a `GET` of that namespace once a second, and each call returns 200.

On the passive hub this stopped after Secret `postgres` in `multicluster-engine`. The repeated request was `GET` of namespace `vm00006`. That namespace was `Terminating`. Velero does not restore into a `Terminating` namespace. It waits until the namespace is gone so it can create it again.

`vm00006` could not finish deleting. Its status condition `NamespaceContentRemaining` still listed a Secret, and `NamespaceFinalizersRemaining` reported finalizer `baremetalhost.metal3.io/secret`. The Secret is named `vm00006` and carries that finalizer. The `baremetalhosts.metal3.io` CRD was installed, and no baremetal-operator was running to clear the finalizer. Other managed-cluster namespaces (`vm00001`, `vm00003`, `vm00004`, `vm00007`) were in the same state. The credentials backup contains secrets for those namespaces, so the restore reaches them and stops on the first `Terminating` one it has to recreate.

Clear those namespaces, then create a new restore. A restore left `InProgress` does not move past the namespace on its own.

### Long gaps that are not a bad Secret

`kube-apiserver` on this hub sets `min-request-timeout: 3600`. With Velero's informer cache left on, a restore can pause for about an hour between groups of items. The process has an established connection only to the Kubernetes service, and the log stops after a `Restored N` line or after `resource cannot be resolved via discovery` for a CRD named in `--restore-resource-priorities`.

Set this on the OADP `DataProtectionApplication` that owns Velero:

```yaml
spec:
  configuration:
    velero:
      disableInformerCache: true
```

The deployment arg becomes `--disable-informer-cache=true` and stays set across pod restarts. This removes the informer sync wait. It does not help the `Terminating` namespace wait above; that wait is a direct `GET` loop.

`oc rollout restart deploy/velero` does not replace the process. OADP reconciles the deployment back. Delete the Velero pod.

If the pod starts while a Velero restore is still `InProgress`, Velero marks that restore `Failed` with `found a restore with status "InProgress" during the server starting`. ACM then sets phase `Error` and `lastMessage` to `Velero restore <name> has failed validation or encountered errors`, including when `validationErrors`, `errors`, and `warnings` are empty. The Velero `failureReason` is the restart text above.

### Remove a stuck restore

1. Delete the ACM `Restore` and the Velero `Restore` objects it created. Leave `Backup`, `Schedule`, and `BackupStorageLocation` objects.
2. If either object stays, it still has a finalizer: `restores.cluster.open-cluster-management.io/finalizer` on the ACM object, and `restores.velero.io/external-resources-finalizer` on the Velero object while the controller is inside that restore. Remove `/metadata/finalizers`.
3. Delete the Velero pod so the controller leaves the old restore loop. Wait until the replacement pod is Ready and no restore objects remain.

Create a new ACM `Restore` with the same spec after the `Terminating` namespaces are gone.
