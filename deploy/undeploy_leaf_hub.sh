#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

acm_namespace=open-cluster-management

echo "using kubeconfig $KUBECONFIG"

curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-spec-sync/$TAG/deploy/leaf-hub-spec-sync.yaml.template" | \
    SYNC_SERVICE_PORT="" IMAGE="" envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/open-cluster-management/leaf-hub-status-sync/$TAG/deploy/leaf-hub-status-sync.yaml.template" | \
    SYNC_SERVICE_PORT="" IMAGE="" envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-sync-service/$TAG/ess/ess.yaml.template" | \
    CSS_HOST="" CSS_PORT="" LISTENING_PORT="" envsubst | kubectl delete -f -

kubectl annotate mch multiclusterhub  --overwrite mch-imageOverridesCM=  -n "$acm_namespace"
kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found
