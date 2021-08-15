#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

css_sync_service_port=9689
ess_sync_service_listening_port=8090

echo "using kubeconfig $KUBECONFIG"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-sync-service/$TAG/ess/ess.yaml.template" | \
    CSS_HOST="$SYNC_SERVICE_HOST" CSS_PORT="$css_sync_service_port" LISTENING_PORT="$ess_sync_service_listening_port" envsubst | kubectl apply -f -
