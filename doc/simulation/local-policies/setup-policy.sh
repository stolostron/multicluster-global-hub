#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail


### This script is used to setup policy and placement for testing
### Usage: ./setup-policy.sh <root-policy-number> [kubeconfig]
if [ $# -ne 2 ]; then
    echo "Usage: $0 <policy_start:policy_end> <KUBECONFIG>"
    exit 1
fi

IFS=':' read -r policy_start policy_end <<< "$1"
KUBECONFIG=$2

echo ">> Generate policy ${policy_start}~${policy_end} on $KUBECONFIG"

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"
CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
kubectl apply -f $REPO_DIR/pkg/testdata/crds/0000_00_policy.open-cluster-management.io_policies.crd.yaml
kubectl apply -f $REPO_DIR/pkg/testdata/crds/0000_00_cluster.open-cluster-management.io_placements.crd.yaml
kubectl apply -f $REPO_DIR/pkg/testdata/crds/0000_03_clusters.open-cluster-management.io_placementdecisions.crd.yaml
max_concurrency=5

source ${CURRENT_DIR}/policy.sh

function generate_replicas_policy() {
  rootpolicy_name=$1
  cluster_start=$2
  cluster_end=$3

  echo ">> Policy ${rootpolicy_name} is propagating to clusters $cluster_start~$cluster_end on $KUBECONFIG"

  # create root policy
  limit_range_policy $rootpolicy_name &

  # create replicas policy: rootpolicy namespace, name and managed cluster
  concurrency=0
  for j in $(seq $cluster_start $cluster_end); do
    cluster_name=managedcluster-${j}
    echo ">> Generating policy ${cluster_name}/${rootpolicy_name} on $KUBECONFIG"

    limit_range_replicas_policy default $rootpolicy_name ${cluster_name} &
    ((concurrency++))

    if [ $j == 1 ]; then
      status="{clustername: $cluster_name, clusternamespace: $cluster_name, compliant: NonCompliant}"
      decision="{clusterName: $cluster_name, reason: ''}"
    else
      status="${status}, {clustername: $cluster_name, clusternamespace: $cluster_name, compliant: NonCompliant}"
      decision="${decision}, {clusterName: $cluster_name, reason: ''}"
    fi
    
    if ((concurrency > max_concurrency)); then
        wait
        concurrency=0
    fi
  done

  wait

  # patch root policy status
  kubectl patch policy $rootpolicy_name -n default --type=merge --subresource status --patch "status: {compliant: NonCompliant, placement: [{placement: placement-$rootpolicy_name, placementBinding: binding-$rootpolicy_name}], status: [${status}]}" &

  # generate placement and placementdecision, each rootpolicy with a placement and placementdescision
  generate_placement default placement-$rootpolicy_name "$decision" &
  
  wait

  echo ">> Policy ${rootpolicy_name} is propagated to clusters $cluster_start~$cluster_end on $KUBECONFIG"
}

for i in $(seq ${policy_start} ${policy_end}); do
  sorted_clusters=$(kubectl get mcl | grep -oE 'managedcluster-[0-9]+' | awk -F"-" '{print $2}' | sort -n)
  # Extract the minimum and maximum cluster numbers
  cluster_start=$(echo "$sorted_clusters" | head -n 1)
  cluster_end=$(echo "$sorted_clusters" | tail -n 1)

  # Check if the root policy is finished
  compliant_status=$(kubectl get policy "rootpolicy-${i}" -n default -o jsonpath="{.status.compliant}" 2>/dev/null)
  if [ "$compliant_status" = "NonCompliant" ]; then 
    echo ">> Policy ${rootpolicy_name} has been propagated to clusters $cluster_start~$cluster_end on $KUBECONFIG"
    continue
  fi

  # create replicas policy: name and managed cluster
  generate_replicas_policy rootpolicy-${i} $cluster_start $cluster_end
done