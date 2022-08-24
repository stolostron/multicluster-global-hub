#!/bin/bash

set -exo pipefail

APISERVER=https://kubernetes.default.svc
SERVICEACCOUNT=/var/run/secrets/kubernetes.io/serviceaccount
NAMESPACE=$(cat ${SERVICEACCOUNT}/namespace)                   # Read this Pod's namespace
TOKEN=$(cat ${SERVICEACCOUNT}/token)
CACERT=${SERVICEACCOUNT}/ca.crt                                # Reference the internal certificate authority (CA)

selectValue='global-hub.open-cluster-management.io/managed-by=multicluster-global-hub-operator'

# ManagedClusterSetBinding
resourcesLink=${APISERVER}/apis/cluster.open-cluster-management.io/v1beta1/managedclustersetbindings 
resources=$(curl -X GET --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" --data-urlencode "labelSelector=$selectValue" ${resourcesLink} | jq .items)
for resource in $(echo $resources | jq -r '.[] | [ .metadata.namespace, .metadata.name ] | join("/managedclustersetbindings/")'); do
  resourceLink=${APISERVER}/apis/cluster.open-cluster-management.io/v1beta1/namespaces/$resource
  curl ${resourceLink} --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" \
    -X PATCH -H 'Content-Type: application/merge-patch+json' \
    -d '{
      "metadata": {
        "finalizers": []
      }
    }' | jq .
echo "pathed resource: $resourceLink"
done

# Placement
resourcesLink=${APISERVER}/apis/cluster.open-cluster-management.io/v1beta1/placements
resources=$(curl -X GET --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" --data-urlencode "labelSelector=$selectValue" ${resourcesLink} | jq .items)
for resource in $(echo $resources | jq -r '.[] | [ .metadata.namespace, .metadata.name ] | join("/placements/")'); do
  resourceLink=${APISERVER}/apis/cluster.open-cluster-management.io/v1beta1/namespaces/$resource
  finalizers=$(curl -X GET --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" ${resourceLink} | jq .metadata.finalizers)
  echo "finalizers: $finalizers"
  finalizers=$( echo $finalizers | jq 'del(.[] | select(. == "global-hub.open-cluster-management.io/resource-cleanup"))' )
  echo "finalizers: $finalizers"

  curl ${resourceLink} --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" \
    -X PATCH -H 'Content-Type: application/merge-patch+json' \
    -d "{
      \"metadata\": {
        \"finalizers\": $finalizers
      }
    }" | jq .
  echo "pathed resource: $resourceLink"
done

