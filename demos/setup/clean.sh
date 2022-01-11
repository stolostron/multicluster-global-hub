#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

cd ../deploy

KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./undeploy_hub_of_hubs_agent.sh
KUBECONFIG=$HUB2_CONFIG LH_ID=hub2 ./undeploy_hub_of_hubs_agent.sh
KUBECONFIG=$TOP_HUB_CONFIG ./undeploy_hub_of_hubs.sh
