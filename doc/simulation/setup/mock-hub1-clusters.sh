#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### Create mock managed clusters on a real hub cluster (no KinD required).
### The managed clusters are named "managedcluster-N" to be compatible
### with the existing setup-policy.sh and rotate-policy.sh scripts.
###
### Usage: ./mock-hub1-clusters.sh <cluster_start:cluster_end> [hub_kubeconfig] [hub_index]
###
### Examples:
###   ./mock-hub1-clusters.sh 1:300
###   ./mock-hub1-clusters.sh 1:300 /tmp/myan-hub1 1
###   ./mock-hub1-clusters.sh 1:300 /path/to/hub2-kubeconfig 2

set -eo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <cluster_start:cluster_end> [hub_kubeconfig] [hub_index]"
    exit 1
fi

IFS=':' read -r cluster_start cluster_end <<< "$1"
HUB_KUBECONFIG="${2:-/tmp/myan-hub1}"
hub_index="${3:-1}"

# Input validation
if ! [[ "$cluster_start" =~ ^[0-9]+$ ]] || ! [[ "$cluster_end" =~ ^[0-9]+$ ]]; then
    echo "Error: cluster range must be integers (got: ${cluster_start}:${cluster_end})" >&2
    exit 1
fi
if (( cluster_start > cluster_end )); then
    echo "Error: cluster_start (${cluster_start}) must be <= cluster_end (${cluster_end})" >&2
    exit 1
fi

echo ">> Creating mock managed clusters ${cluster_start}~${cluster_end} on hub${hub_index}"
echo ">> Hub kubeconfig: ${HUB_KUBECONFIG}"

# Maximum parallel jobs
MAX_CONCURRENT_JOBS=20
CURRENT_JOBS=0

function create_managed_cluster() {
    local cluster_id=$1
    local cluster_name=$2
    local kubeconfig=$3

    # Create ManagedCluster resource
    kubectl --kubeconfig "$kubeconfig" --request-timeout=30s apply -f - 2>/dev/null <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: $cluster_name
spec:
  hubAcceptsClient: true
EOF

    # Patch status: clusterClaims (unique UUID for cluster ID)
    kubectl --kubeconfig "$kubeconfig" --request-timeout=30s patch ManagedCluster "$cluster_name" \
        --type=merge --subresource status \
        --patch "status: {clusterClaims: [{name: id.k8s.io, value: ${cluster_id}}]}" 2>/dev/null

    # Patch status: conditions (hub accepted, joined, available)
    kubectl --kubeconfig "$kubeconfig" --request-timeout=30s patch ManagedCluster "$cluster_name" \
        --type=merge --subresource status \
        --patch 'status: {conditions: [
          {lastTransitionTime: "2024-01-01T00:00:00Z", message: "Accepted by hub cluster admin", reason: "HubClusterAdminAccepted", status: "True", type: "HubAcceptedManagedCluster"},
          {lastTransitionTime: "2024-01-01T00:00:01Z", message: "Managed cluster joined", reason: "ManagedClusterJoined", status: "True", type: "ManagedClusterJoined"},
          {lastTransitionTime: "2024-01-01T00:00:01Z", message: "ManagedClusterAvailable", reason: "ManagedClusterAvailable", status: "True", type: "ManagedClusterConditionAvailable"}
        ]}' 2>/dev/null

    echo ">> Created ${cluster_name} (hub${hub_index}, id: ${cluster_id})"
}

export -f create_managed_cluster

for j in $(seq "$cluster_start" "$cluster_end"); do
    cluster_name="managedcluster-${j}"
    # Build UUID: 0000000H-0000-0000-0000-00000000000N  (H=hub_index, N=cluster_index)
    cluster_id=$(printf "%08d-0000-0000-0000-%012d" "${hub_index}" "${j}")

    create_managed_cluster "${cluster_id}" "${cluster_name}" "${HUB_KUBECONFIG}" &

    ((CURRENT_JOBS++)) || true
    if ((CURRENT_JOBS >= MAX_CONCURRENT_JOBS)); then
        wait
        CURRENT_JOBS=0
    fi
done

wait

total=$((cluster_end - cluster_start + 1))
echo ""
echo ">> Done. Created ${total} mock managed clusters (managedcluster-${cluster_start}~managedcluster-${cluster_end}) on hub${hub_index}"
echo ">> Verify with: kubectl --kubeconfig ${HUB_KUBECONFIG} get mcl | grep managedcluster | wc -l"
