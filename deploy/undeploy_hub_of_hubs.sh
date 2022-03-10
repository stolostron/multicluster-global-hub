#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"
branch=$TAG
if [ $TAG == "latest" ]; then
  branch="main"
fi

function uninstall_component() {
    rm -rf $1
    git clone https://github.com/stolostron/$1.git
    cd $1
    git checkout $2
    $3
    cd ..
    rm -rf $1
}

acm_namespace=open-cluster-management

helm uninstall console-chart -n "$acm_namespace" 2> /dev/null || true
helm uninstall grc -n "$acm_namespace" 2> /dev/null || true
kubectl annotate mch multiclusterhub mch-pause=false -n "$acm_namespace" --overwrite

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-nonk8s-api/$branch/deploy/ingress.yaml.template" |
    COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-nonk8s-api/$branch/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-rbac/$branch/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-rbac envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

kubectl delete secret opa-data -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-spec-sync/$branch/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-status-sync/$branch/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-status-sync envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-spec-transport-bridge/$branch/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-status-transport-bridge/$branch/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

kubectl delete secret hub-of-hubs-database-secret-transport-bridge-secret -n "$acm_namespace" --ignore-not-found

# remove HoH config resources in case it exists
hoh_config_crd_exists=$(kubectl get crd configs.hub-of-hubs.open-cluster-management.io --ignore-not-found)
if [[ ! -z "$hoh_config_crd_exists" ]]; then
  # replace the existing HoH config to make sure no finalizer is found
  kubectl replace -f "https://raw.githubusercontent.com/stolostron/hub-of-hubs-crds/$branch/cr-examples/hub-of-hubs.open-cluster-management.io_config_cr.yaml" -n hoh-system
  # delete the HoH config CRD
  kubectl delete -f "https://raw.githubusercontent.com/stolostron/hub-of-hubs-crds/$branch/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
fi

kubectl annotate mch multiclusterhub --overwrite mch-imageOverridesCM= -n "$acm_namespace"
kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found

kubectl delete namespace hoh-system --ignore-not-found

# delete kafka namespace if exists - this will also delete all living resources inside the kafka namespace
kubectl delete namespace kafka --ignore-not-found

# delete sync-service namespace if exists - this will also delete all living resources inside the sync-service namespace
kubectl delete namespace sync-service --ignore-not-found

# uninstall PGO
uninstall_component "hub-of-hubs-postgresql" "$branch" "kubectl delete -k ./pgo/install -k ./pgo/high-availability --ignore-not-found"

# uninstall hub cluster controller
uninstall_component "hub-cluster-controller" "$branch" "kubectl delete -k ./deploy --ignore-not-found"

# uninstall hub-of-hubs addon controller
uninstall_component "hub-of-hubs-addon" "$branch" "kubectl delete -k ./deploy --ignore-not-found"
