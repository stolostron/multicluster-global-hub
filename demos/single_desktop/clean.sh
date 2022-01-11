#!/bin/bash

# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o errexit
set -o nounset

kubectl label managedcluster cluster0 env- --kubeconfig $HUB1_CONFIG
kubectl label managedcluster cluster3 env- --kubeconfig $HUB1_CONFIG

kubectl label managedcluster cluster7 env- --kubeconfig $HUB2_CONFIG
kubectl label managedcluster cluster8 env- --kubeconfig $HUB2_CONFIG
kubectl label managedcluster cluster9 env- --kubeconfig $HUB2_CONFIG

kubectl delete -f https://raw.githubusercontent.com/stolostron/hub-of-hubs/main/demos/policy-psp.yaml --kubeconfig $TOP_HUB_CONFIG --ignore-not-found=true
