#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -o pipefail

### This script is used to setup policy and placement for testing
### Usage: ./cleanup-policy.sh <root-policy-number> <replicas-number/cluster-number>

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
source ${CURRENT_DIR}/policy.sh

for i in $(seq 1 $1)
do
  # delete root policy
  kubectl delete policy rootpolicy-${i} -n default 
  # delete replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq 1 $2) 
  do
    kubectl delete policy default.rootpolicy-${i} -n managedcluster-${j}
  done

  # delete placement and placementdecision
  kubectl delete placement placement-roopolicy-${i} -n default
  kubectl delete placementdecision placement-roopolicy-${i}-1 -n default 
done