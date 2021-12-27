#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"

acm_namespace=open-cluster-management

helm uninstall console-chart -n "$acm_namespace" 2> /dev/null || true
kubectl annotate mch multiclusterhub mch-pause=false -n "$acm_namespace" --overwrite

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/ingress.yaml.template" |
    COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-nonk8s-api/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-nonk8s-api envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-rbac/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG=$TAG COMPONENT=hub-of-hubs-rbac envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
kubectl delete secret opa-data -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-spec-sync envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-sync/$TAG/deploy/operator.yaml.template" |
    REGISTRY=quay.io/open-cluster-management-hub-of-hubs IMAGE_TAG="$TAG" COMPONENT=hub-of-hubs-status-sync envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

kubectl delete secret hub-of-hubs-database-secret -n "$acm_namespace" --ignore-not-found

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-spec-transport-bridge/$TAG/deploy/hub-of-hubs-spec-transport-bridge.yaml.template" |
    envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found
curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-status-transport-bridge/$TAG/deploy/hub-of-hubs-status-transport-bridge.yaml.template" |
    envsubst | kubectl delete -f - -n "$acm_namespace" --ignore-not-found

kubectl delete secret hub-of-hubs-database-secret-transport-bridge-secret -n "$acm_namespace" --ignore-not-found

# remove HoH config resources in case it exists
hoh_config_crd_exists=$(kubectl get crd configs.hub-of-hubs.open-cluster-management.io --ignore-not-found)
if [[ ! -z "$hoh_config_crd_exists" ]]; then
  # replace the existing HoH config to make sure no finalizer is found
  kubectl replace -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/cr-examples/hub-of-hubs.open-cluster-management.io_config_cr.yaml" -n hoh-system
  # delete the HoH config CRD
  kubectl delete -f "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-crds/$TAG/crds/hub-of-hubs.open-cluster-management.io_config_crd.yaml"
fi

kubectl annotate mch multiclusterhub --overwrite mch-imageOverridesCM= -n "$acm_namespace"
kubectl delete configmap custom-repos -n "$acm_namespace" --ignore-not-found

kubectl delete namespace hoh-system --ignore-not-found

# delete kafka namespace if exists - this will also delete all living resources inside the kafka namespace
kubectl delete namespace kafka --ignore-not-found

# uninstall PGO
rm -rf hub-of-hubs-postgresql
git clone https://github.com/open-cluster-management/hub-of-hubs-postgresql
cd hub-of-hubs-postgresql
git checkout $TAG
kubectl delete -k ./pgo/high-availability --ignore-not-found
kubectl delete -k ./pgo/install --ignore-not-found
cd ..
rm -rf hub-of-hubs-postgresql
