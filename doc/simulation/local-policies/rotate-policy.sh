#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

### This script is used to setup policy and placement for testing
### Usage: ./rotate-policy.sh <root-policy-number> <replicas-number/cluster-number> Compliant/NonCompliant

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)

count=0

for i in $(seq 1 $1)
do
  # path replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq 1 $2) 
  do
    root_policy_namespace=default
    root_policy_name=rootpolicy-${i}
    cluster_name=managedcluster-${j}

    if [ $j == 1 ]
    then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: NonCompliant}"
    fi

    if [[ $3 == "Compliant" ]]; then 
      # patch replicas policy status to compliant
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: Compliant, details: [{compliant: Compliant, history: [{eventName: $root_policy_namespace.$root_policy_name.$cluster_name, message: Compliant; notification - limitranges container-mem-limit-range found as specified in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}"
    else 
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: NonCompliant, details: [{compliant: NonCompliant, history: [{eventName: $root_policy_namespace.$root_policy_name.$cluster_name, message: NonCompliant; violation - limitranges container-mem-limit-range not found in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}"
    fi

    if [ $j == 1 ];then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $3}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $3}"
    fi

    count=$(( $count + 1 ))
    if (( count == 5000 )); then
      sleep 600
    else
      count=0
    fi
  done

 
  # patch root policy status
  kubectl patch policy rootpolicy-${i} -n default --type=merge --subresource status --patch "status: {compliant: $3, placement: [{placement: placement-roopolicy-${i}, placementBinding: binding-roopolicy-${i}}], status: [${status}]}"
done