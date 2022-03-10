#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

css_sync_service_port=${CSS_SYNC_SERVICE_PORT:-9689}
ess_sync_service_listening_port=8090

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-sync-service/$branch/ess/ess.yaml.template" |
    CSS_HOST="$CSS_SYNC_SERVICE_HOST" CSS_PORT="$css_sync_service_port" LISTENING_PORT="$ess_sync_service_listening_port" envsubst | kubectl apply -f -
