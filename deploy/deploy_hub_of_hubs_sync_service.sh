#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

css_sync_service_port=${CSS_SYNC_SERVICE_PORT:-9689}

curl -s "https://raw.githubusercontent.com/stolostron/hub-of-hubs-sync-service/$branch/css/css.yaml.template" |
    CSS_PORT="$css_sync_service_port" envsubst | kubectl apply -f -
