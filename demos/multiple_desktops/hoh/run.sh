#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

########################
# include the magic
########################
. ../../demo-magic.sh


# hide the evidence
clear

pe 'kubectl get managedcluster -o=jsonpath="{.items}{\"\n\"}"'

pe 'kubectl apply -f https://raw.githubusercontent.com/stolostron/hub-of-hubs/main/demos/policy-psp.yaml'
