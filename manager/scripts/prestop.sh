#!/bin/bash

set -exo pipefail

APISERVER=https://kubernetes.default.svc
SERVICEACCOUNT=/var/run/secrets/kubernetes.io/serviceaccount
NAMESPACE=$(cat ${SERVICEACCOUNT}/namespace)                   # Read this Pod's namespace
TOKEN=$(cat ${SERVICEACCOUNT}/token)
CACERT=${SERVICEACCOUNT}/ca.crt                                # Reference the internal certificate authority (CA)

LABEL=global-hub.open-cluster-management.io%2Fmanaged-by%3Dmulticluster-global-hub-operator

# ManagedClusterSetBinding
resourcesLink=apis/cluster.open-cluster-management.io/v1beta1/namespaces/${NAMESPACE}/managedclustersetbindings
resourceNames=$(curl --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" ${APISERVER}/${resourcesLink}?labelSelector=$LABEL | jq .items[].metadata.name)
for resource in $resourceNames; do
  resource=$(echo $resource |sed 's/\"//g')
  resourceLink=$resourcesLink/$resource
  curl ${APISERVER}/${resourceLink} --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" \
    -X PATCH -H 'Content-Type: application/merge-patch+json' \
    -d '{
      "metadata": {
        "finalizers": []
      }
    }' | jq .
echo "pathed resource: $resourceLink"
done

# Placement
resourcesLink=apis/cluster.open-cluster-management.io/v1beta1/namespaces/${NAMESPACE}/placements
resourceNames=$(curl --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" ${APISERVER}/${resourcesLink}?labelSelector=$LABEL | jq .items[].metadata.name)
for resource in $resourceNames; do
  resource=$(echo $resource |sed 's/\"//g')
  resourceLink=$resourcesLink/$resource
  curl ${APISERVER}/${resourceLink} --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" \
    -X PATCH -H 'Content-Type: application/merge-patch+json' \
    -d '{
      "metadata": {
        "finalizers": []
      }
    }' | jq .
echo "pathed resource: $resourceLink"
done
