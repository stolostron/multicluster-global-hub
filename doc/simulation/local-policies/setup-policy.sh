#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail


### This script is used to setup policy and placement for testing
### Usage: ./setup-policy.sh <root-policy-number> <replicas-number/cluster-number> [kubeconfig]

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
KUBECONFIG=$3
FROM_POLICY_IDX=${FROM_POLICY_IDX:-1}

source ${CURRENT_DIR}/policy.sh

function generate_replicas_policy() {
  rootpolicy_name=$1
  cluster_num=$2

  # create root policy
  limit_range_policy $rootpolicy_name &

  # create replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq 1 $cluster_num); do
    echo "Generating managedcluster-${j}/${rootpolicy_name} on $KUBECONFIG"

    limit_range_replicas_policy default $rootpolicy_name managedcluster-${j} 
    if [ $j == 1 ]; then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
      decision="{clusterName: managedcluster-${j}, reason: ''}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
      decision="${decision}, {clusterName: managedcluster-${j}, reason: ''}"
    fi
  done

  # patch root policy status
  kubectl patch policy $rootpolicy_name -n default --type=merge --subresource status --patch "status: {compliant: NonCompliant, placement: [{placement: placement-roopolicy-${i}, placementBinding: binding-roopolicy-${i}}], status: [${status}]}" &

  # generate placement and placementdecision, each rootpolicy with a placement and placementdescision
  generate_placement default placement-$rootpolicy_name &
  # patch placementdecision status
  kubectl patch placementdecision placement-${rootpolicy_name}-1 -n default --type=merge --subresource status --patch "status: {decisions: [${decision}]}" &

  wait

  echo "Rootpolicy ${rootpolicy_name} propagate to $cluster_num clusters on $KUBECONFIG"
}

for i in $(seq $FROM_POLICY_IDX $1); do
  # create replicas policy: name and managed cluster
  generate_replicas_policy rootpolicy-${i} $2
done