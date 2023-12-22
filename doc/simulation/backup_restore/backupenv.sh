#!/usr/bin/env bash

rm -rf run
mkdir run
cd run
cp -r ../backup ./
cp -r ../common ./


sed -i 's/<AWS_BUCKET>/'$AWS_BUCKET'/g' common/data-protection-app.yaml

sed -i 's/<AWS_ACCESS_KEY_ID>/'$AWS_ACCESS_KEY_ID'/g' common/credentials-velero
sed -i 's/<AWS_ACCESS_KEY_SECRET>/'$AWS_SECRET_ACCESS_KEY'/g' common/credentials-velero

sed -i 's/<AWS_ACCESS_KEY_ID>/'$AWS_ACCESS_KEY_ID'/g' backup/restic-secret.yaml
sed -i 's/<AWS_ACCESS_KEY_SECRET>/'$AWS_SECRET_ACCESS_KEY'/g' backup/restic-secret.yaml
sed -i 's/<RESTIC_REPO>/'$RESTIC_REPO'/g' backup/restic-secret.yaml
sed -i 's/<RESTIC_PASSWD>/'$RESTIC_PASSWD'/g' backup/restic-secret.yaml

##clean up
oc delete -f common/backup-policy.yaml
oc delete secret cloud-credentials -n open-cluster-management-backup
oc delete -f backup/
oc delete -f backup/globalhub-custom/
sleep 20

oc apply -f common/mch.yaml
oc apply -f common/backup-policy.yaml
oc apply -f common/mce.yaml

sleep 20

oc create ns open-cluster-management-backup

oc create secret generic cloud-credentials -n open-cluster-management-backup --from-file cloud=common/credentials-velero
oc apply -f common/data-protection-app.yaml


#oc apply -f backup/schedule-acm.yaml
#oc apply -f backup/restic-secret.yaml
#oc apply -f backup/volsync-config.yaml

oc apply -f backup/
oc apply -f backup/globalhub-custom/

cd ..
