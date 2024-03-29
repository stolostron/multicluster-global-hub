#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)

# Check if the script is provided with the correct number of positional parameters
if [ $# -lt 2 ]; then
    echo "Usage: $0 <policy_start:policy_end> <Compliant/NonCompliant> <KUBECONFIG>"
    exit 1
fi

# Parse the parameter using the delimiter ":"
IFS=':' read -r policy_start policy_end <<< "$1"
compliance_state=$2
export KUBECONFIG=$3
concurrent="${4:-1}"

sorted_clusters=$(kubectl get mcl | grep -oE 'managedcluster-[0-9]+' | awk -F"-" '{print $2}' | sort -n)
cluster_start=$(echo "$sorted_clusters" | head -n 1)
cluster_end=$(echo "$sorted_clusters" | tail -n 1)

echo ">> KUBECONFIG=$KUBECONFIG"
echo ">> Rotating Policy $policy_start~$policy_end to $compliance_state on cluster $cluster_start~$cluster_end"

random_number=$(shuf -i 10000-99999 -n 1)

function update_cluster_policies() {
  root_policy_namespace=default
  root_policy_name=$1
  root_policy_status=$2

  echo ">> Rotating $root_policy_name to $root_policy_status on cluster $cluster_start~$cluster_end"

  count=0
  # path replicas policy: rootpolicy namespace, name and managed cluster
  for j in $(seq $cluster_start $cluster_end); do

    cluster_name=managedcluster-${j}
    event_name=$root_policy_namespace.$root_policy_name.$cluster_name.$random_number
    if [[ $root_policy_status == "Compliant" ]]; then 
      # patch replicas policy status to compliant
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: Compliant, details: [{compliant: Compliant, history: [{eventName: $event_name, message: Compliant; notification - limitranges container-mem-limit-range found as specified in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}" &
    else 
      kubectl patch policy $root_policy_namespace.$root_policy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: NonCompliant, details: [{compliant: NonCompliant, history: [{eventName: $event_name, message: NonCompliant; violation - limitranges container-mem-limit-range not found in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}" &
    fi

    if [ $j == 1 ];then
      status="{clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $root_policy_status}"
    else
      status="${status}, {clustername: managedcluster-${j}, clusternamespace: managedcluster-${j}, compliant: $root_policy_status}"
    fi

    count=$(( $count + 1 ))
    if (( count == concurrent )); then
      wait
      count=0
    fi
  done

  wait

  # patch root policy status
  kubectl patch policy $root_policy_name -n $root_policy_namespace --type=merge --subresource status --patch "status: {compliant: $root_policy_status, placement: [{placement: placement-$root_policy_name, placementBinding: binding-${root_policy_name}}], status: [${status}]}"
} 

for i in $(seq $policy_start $policy_end)
do
  # path replicas policy: rootpolicy namespace, name and managed cluster
  update_cluster_policies "rootpolicy-${i}" $compliance_state
done