#!/usr/bin/env bash


#Install ACM and Globalhub Operator

rm -rf run
mkdir run
cd run
cp -r ../restore ./
cp -r ../common ./

sed -i 's/<AWS_ACCESS_KEY_ID>/'$AWS_ACCESS_KEY_ID'/g' common/credentials-velero
sed -i 's/<AWS_ACCESS_KEY_SECRET>/'$AWS_SECRET_ACCESS_KEY'/g' common/credentials-velero
sed -i 's/<AWS_BUCKET>/'$AWS_BUCKET'/g' common/data-protection-app.yaml

git clone https://github.com/open-cluster-management-io/policy-collection.git


##clean up
oc delete -f common/backup-policy.yaml
oc delete secret cloud-credentials -n open-cluster-management-backup
oc delete -f restore/
oc delete -f policy-collection/community/CM-Configuration-Management/acm-hub-pvc-backup/


sleep 20

oc create ns open-cluster-management-backup

oc apply -f common/mch.yaml
oc apply -f policy-collection/community/CM-Configuration-Management/acm-hub-pvc-backup/
oc apply -f common/mce.yaml
oc apply -f common/msb.yaml

sleep 200

oc create secret generic cloud-credentials -n open-cluster-management-backup --from-file cloud=common/credentials-velero
oc apply -f common/data-protection-app.yaml


oc apply -f restore/restore.yaml
