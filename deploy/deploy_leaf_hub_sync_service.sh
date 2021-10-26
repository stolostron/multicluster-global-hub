#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

TAG=${TAG:="v0.1.0"}

css_sync_service_host=${CSS_SYNC_SERVICE_HOST:="ec2-107-21-78-35.compute-1.amazonaws.com"}
css_sync_service_port=${CSS_SYNC_SERVICE_PORT:-9689}
ess_sync_service_listening_port=8090

echo "using kubeconfig $KUBECONFIG"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-sync-service/$TAG/ess/ess.yaml.template" |
    CSS_HOST="$css_sync_service_host" CSS_PORT="$css_sync_service_port" LISTENING_PORT="$ess_sync_service_listening_port" envsubst | kubectl apply -f -
