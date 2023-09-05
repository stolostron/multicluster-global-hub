#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

### This script is used to setup policy and placement for testing
### Usage: ./setup-policy.sh <root-policy-number> <replicas-number/cluster-number>

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
source ${CURRENT_DIR}/policy.sh

for i in $(seq 1 $1)
do
  # create root policy
  limit_range_policy rootpolicy-${i}
  # create replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq 1 $2) 
  do
    limit_range_replicas_policy default rootpolicy-${i} managedcluster-${j}
    if [ $j == 1 ]
    then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
      decision="{clusterName: managedcluster-${j}, reason: ''}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
      decision="${decision}, {clusterName: managedcluster-${j}, reason: ''}"
    fi
  done

  # patch root policy status
  kubectl patch policy rootpolicy-${i} -n default --type=merge --subresource status --patch "status: {compliant: NonCompliant, placement: [{placement: placement-roopolicy-${i}, placementBinding: binding-roopolicy-${i}}], status: [${status}]}"

  # generate placement and placementdecision, each rootpolicy with a placement and placementdescision
  generate_placement default placement-roopolicy-${i}
  # patch placementdecision status
  kubectl patch placementdecision placement-roopolicy-${i}-1 -n default --type=merge --subresource status --patch "status: {decisions: [${decision}]}"
done