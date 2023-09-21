#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

### This script is used to setup policy and placement for testing
### Usage: ./rotate-policy.sh <root-policy-number> <replicas-number/cluster-number> Compliant/NonCompliant

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)


function update_cluster_policies() {
  root_policy_namespace=default
  root_policy_name=$1
  cluster_num=$2
  count=0

  # path replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq 1 ${cluster_num}); do
    cluster_name=managedcluster-${j}

    if [[ $3 == "Compliant" ]]; then 
      # patch replicas policy status to compliant
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: Compliant, details: [{compliant: Compliant, history: [{eventName: $root_policy_namespace.$root_policy_name.$cluster_name, message: Compliant; notification - limitranges container-mem-limit-range found as specified in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}" &
    else 
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: NonCompliant, details: [{compliant: NonCompliant, history: [{eventName: $root_policy_namespace.$root_policy_name.$cluster_name, message: NonCompliant; violation - limitranges container-mem-limit-range not found in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}" &
    fi

    if [ $j == 1 ];then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $3}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $3}"
    fi

    count=$(( $count + 1 ))
    if (( count == 10 )); then
      wait
      count=0
    fi
  done

  wait

  # patch root policy status
  kubectl patch policy $root_policy_name -n $root_policy_namespace --type=merge --subresource status --patch "status: {compliant: $3, placement: [{placement: placement-$root_policy_name, placementBinding: binding-${root_policy_name}}], status: [${status}]}"
} 

for i in $(seq 1 $1)
do
  # path replicas policy: rootpolicy namespace, name and managed cluster
  update_cluster_policies "rootpolicy-${i}" $2 $3 
done