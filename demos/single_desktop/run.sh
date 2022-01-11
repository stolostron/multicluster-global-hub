#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

########################
# include the magic
########################
. ./demo-magic/demo-magic.sh


# hide the evidence
clear

pe 'kubectl get managedcluster --kubeconfig $TOP_HUB_CONFIG -o=jsonpath="{.items}{\"\n\"}"'

pe 'kubectl label managedcluster cluster0 env=production --kubeconfig $HUB1_CONFIG'
pe 'kubectl label managedcluster cluster3 env=production --kubeconfig $HUB1_CONFIG'

pe 'kubectl label managedcluster cluster7 env=production --kubeconfig $HUB2_CONFIG'
pe 'kubectl label managedcluster cluster8 env=production --kubeconfig $HUB2_CONFIG'
pe 'kubectl label managedcluster cluster9 env=production --kubeconfig $HUB2_CONFIG'

pe 'kubectl apply -f https://raw.githubusercontent.com/stolostron/hub-of-hubs/main/demos/policy-psp.yaml --kubeconfig $TOP_HUB_CONFIG'
