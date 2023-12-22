#!/usr/bin/env bash


#Install ACM and Globalhub Operator

sed -i 's/<AWS_ACCESS_KEY_ID>/c'$AWS_ACCESS_KEY_ID'/g' ../credentials-velero
sed -i 's/<AWS_ACCESS_KEY_SECRET>/c'$AWS_SECRET_ACCESS_KEY'/g' ../credentials-velero
sed -i 's/<AWS_BUCKET>/'$AWS_BUCKET'/g' common/data-protection-app.yaml

oc apply -f ../mch.yaml
oc apply -f ../backup-policy.yaml

sleep 20

oc create project open-cluster-management-backup
oc create secret generic cloud-credentials -n open-cluster-management-backup --from-file cloud=../credentials-velero
oc apply -f ../data-protection-app.yaml


oc apply -f restore.yaml

