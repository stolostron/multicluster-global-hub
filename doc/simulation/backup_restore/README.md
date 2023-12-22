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

cd backup
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


## Stop backup
```
oc delete -f backup/schedule-acm.yaml
```


# Restore:
## Install ACM and globalhub operator(do not include mgh)
## Start restore
```sh
export AWS_ACCESS_KEY_ID=''
export AWS_SECRET_ACCESS_KEY=''
export AWS_BUCKET=''

cd backup
./restoreenv.sh
```
## Check restore
```sh
oc get restore -A
```
