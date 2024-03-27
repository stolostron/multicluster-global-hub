#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### Usage: ./setup-policy.sh <hub-cluster-number> <root-policy-number> <replicas-number/cluster-number>

set -eo pipefail

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"
START_HUB_IDX=${START_HUB_IDX:-1}
END_HUB_IDX=$1

START_POLICY_IDX=${START_POLICY_IDX:-1}

echo "Generate policies($START_POLICY_IDX - $2) on hub${START_HUB_IDX} to hub${END_HUB_IDX}"

source ${REPO_DIR}/doc/simulation/local-policies/policy.sh

cluster_dir=${REPO_DIR}/doc/simulation/kubeconfig
mkdir -p ${cluster_dir}

for i in $(seq $START_HUB_IDX $END_HUB_IDX); do
    hub_cluster=hub$i
    root_policy_num=$2
    cluster_num=$3
    kubeconfig="${cluster_dir}/${hub_cluster}"
    
    bash ${REPO_DIR}/doc/simulation/local-policies/setup-policy.sh $root_policy_num $cluster_num $kubeconfig &
done

wait

# printing the clusters
echo "Access the clusters:"
for i in $(seq $START_HUB_IDX $END_HUB_IDX); do
    cluster=hub${i}
    kubeconfig="${cluster_dir}/${cluster}"
    echo "export KUBECONFIG=${kubeconfig}"
done