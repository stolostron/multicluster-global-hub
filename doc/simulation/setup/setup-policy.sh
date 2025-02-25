#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### Usage: ./setup-policy.sh <hub-cluster-number> <root-policy-number> <replicas-number/cluster-number>

set -eo pipefail

# Check if the script is provided with the correct number of positional parameters
if [ $# -ne 2 ]; then
    echo "Usage: $0 <hub_start:hub_end> <policy_start:policy_end>"
    exit 1
fi

# Parse the parameter using the delimiter ":"
IFS=':' read -r hub_start hub_end <<< "$1"
IFS=':' read -r policy_start policy_end <<< "$2"

echo ">> Generate policy ${policy_start}~${policy_end} on hub ${hub_start}~${hub_end}"

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"
export KUBECONFIG=${KUBECONFIG}
source ${REPO_DIR}/doc/simulation/setup/local-policies/policy.sh
cluster_dir=${REPO_DIR}/doc/simulation/kubeconfig
mkdir -p ${cluster_dir}

for i in $(seq $hub_start $hub_end); do
    hub_cluster=hub$i
    kubeconfig="${cluster_dir}/${hub_cluster}"
    bash ${REPO_DIR}/doc/simulation/setup/local-policies/setup-policy.sh $2 $kubeconfig &
done

wait

# printing the clusters
echo "Access the clusters:"
for i in $(seq $hub_start $hub_end); do
    cluster=hub${i}
    kubeconfig="${cluster_dir}/${cluster}"
    echo "export KUBECONFIG=${kubeconfig}"
done