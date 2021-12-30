#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

echo "using kubeconfig $KUBECONFIG"

curl -s "https://raw.githubusercontent.com/open-cluster-management/hub-of-hubs-sync-service/v0.2.0/ess/ess.yaml.template" | \
    CSS_HOST="" CSS_PORT="" LISTENING_PORT="" envsubst | kubectl delete -f -
