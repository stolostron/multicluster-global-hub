#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### Usage: ./setup-policy.sh <hub-cluster-number> <root-policy-number> <replicas-number/cluster-number>

set -eo pipefail

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"
source ${REPO_DIR}/doc/simulation/local-policies/policy.sh

cluster_dir=${REPO_DIR}/doc/simulation/kubeconfig
mkdir -p ${cluster_dir}

for i in $(seq 1 $1); do
    hub_cluster=hub$i
    root_policy_num=$2
    cluster_num=$3
    kubeconfig="${cluster_dir}/${hub_cluster}"
    
    bash ${REPO_DIR}/doc/simulation/local-policies/setup-policy.sh $root_policy_num $cluster_num $kubeconfig &
done

wait

# printing the clusters
echo "Access the clusters:"
for i in $(seq 1 $1); do
    cluster=hub${i}
    kubeconfig="${cluster_dir}/${cluster}"
    echo "export KUBECONFIG=${kubeconfig}"
done