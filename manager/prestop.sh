#!/bin/bash

set -exo pipefail

# Point to the internal API server hostname
APISERVER=https://kubernetes.default.svc

# Path to ServiceAccount token
SERVICEACCOUNT=/var/run/secrets/kubernetes.io/serviceaccount

# Read this Pod's namespace
NAMESPACE=$(cat ${SERVICEACCOUNT}/namespace)

# Read the ServiceAccount bearer token
TOKEN=$(cat ${SERVICEACCOUNT}/token)

# Reference the internal certificate authority (CA)
CACERT=${SERVICEACCOUNT}/ca.crt



resourceLink=apis/cluster.open-cluster-management.io/v1beta1/namespaces/open-cluster-management/managedclustersetbindings

# Delete the validatingwebhookconfiguration with TOKEN
curl --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" ${APISERVER}/${resourceLink}?labelSelector=global-hub.open-cluster-management.io%2Fmanaged-by%3Dmulticluster-global-hub-operator

