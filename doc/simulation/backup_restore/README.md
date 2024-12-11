ACM Official doc:
https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.8/html/business_continuity/business-cont-overview#backup-intro

Prerequest for this script:
1. [Prepare for aws bucket](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html-single/backup_and_restore/index#installing-oadp-aws)
2. [Prepare restic repo](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html#amazon-s3)


# Backup
## Install ACM and Globalhub(include mgh cr)
## Start backup
```shell
export AWS_ACCESS_KEY_ID=''
export AWS_SECRET_ACCESS_KEY=''
export AWS_BUCKET=''

export RESTIC_REPO=''
export  RESTIC_PASSWD=''

./backupenv.sh
```
## check backup successful

```
oc get backup -A
[sh]# oc get backup -A
NAMESPACE                        NAME                                            AGE
open-cluster-management-backup   acm-credentials-schedule-20231218053605         14m
open-cluster-management-backup   acm-managed-clusters-schedule-20231218053605    14m
open-cluster-management-backup   acm-resources-generic-schedule-20231218053605   14m
open-cluster-management-backup   acm-resources-schedule-20231218053605           14m
open-cluster-management-backup   acm-validation-policy-schedule-20231218053605   14m

```
More details about these items could be found [here](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.8/html/business_continuity/business-cont-overview#schedule-backup)

```
[sh]# oc get replicationsource -A
NAMESPACE                 NAME                                            SOURCE                                          LAST SYNC              DURATION          NEXT SYNC
multicluster-global-hub   postgresdb-multicluster-global-hub-postgresql-0   postgresdb-multicluster-global-hub-postgresql-0   2024-01-05T07:15:24Z   1m47.599200873s   2024-01-05T08:00:00Z
```

## Stop backup
```
oc delete -f backup/schedule-acm.yaml
```

# Restore:
## Install ACM and globalhub operator(do not include mgh)
## Start restore (passive)
```sh
export AWS_ACCESS_KEY_ID=''
export AWS_SECRET_ACCESS_KEY=''
export AWS_BUCKET=''

./restoreenv.sh

```
## Check restore
```sh
oc get restores.velero.io -A
NAMESPACE                        NAME                                                                     AGE
open-cluster-management-backup   restore-acm-passive-sync-acm-credentials-schedule-20240105071259         4m33s
open-cluster-management-backup   restore-acm-passive-sync-acm-resources-generic-schedule-20240105071259   4m33s
open-cluster-management-backup   restore-acm-passive-sync-acm-resources-schedule-20240105071259           4m33s
```
```sh
oc get ReplicationDestination
NAME                                                 LAST SYNC              DURATION        NEXT SYNC
b-multicluster-global-hub-postgresql-020240105091547   2024-01-05T09:21:11Z   39.49297454s
```

```sh
oc get pvc -A
NAMESPACE                 NAME                                                               STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
multicluster-global-hub   postgresdb-multicluster-global-hub-postgresql-0                      Bound    pvc-a7aebea3-8d86-40df-a61e-fdf2ac95d0ef   25Gi       RWO            gp3-csi        7m9s
multicluster-global-hub   volsync-b-multicluster-global-hub-postgresql-020240105091547-cache   Bound    pvc-2376eeaf-dc32-4326-9c7b-d5ede983dfe1   1Gi        RWO            gp3-csi        6m59s
```
