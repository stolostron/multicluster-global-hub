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

pe 'kubectl get managedclusters'

pe 'kubectl label managedcluster cluster7 env=production'
pe 'kubectl label managedcluster cluster8 env=production'
pe 'kubectl label managedcluster cluster9 env=production'
