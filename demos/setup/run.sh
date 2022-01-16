#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

########################
# include the magic
########################
. ../demo-magic.sh

# hide the evidence
clear

cd ../../deploy

pe 'KUBECONFIG=$TOP_HUB_CONFIG ./deploy_hub_of_hubs.sh'
pe 'KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./deploy_hub_of_hubs_agent.sh'
pe 'KUBECONFIG=$HUB2_CONFIG LH_ID=hub2 ./deploy_hub_of_hubs_agent.sh'
