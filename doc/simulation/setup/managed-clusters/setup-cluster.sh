#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### This script is used to setup managedclusters for testing
### Usage: ./setup-cluster.sh <cluster-number>

set -eo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"

if [ $# -lt 2 ]; then
  cluster_id_prefix="1"  # Set a default value of "1" if $2 is not provided
else
  cluster_id_prefix="$2"  # Use the provided value of $2
fi

# create the mcl crd
kubectl apply -f $REPO_DIR/test/manifest/crd/0000_00_cluster.open-cluster-management.io_managedclusters.crd.yaml

# creating the simulated managedcluster
for i in $(seq 1 $1)
do
    echo "Creating Simulated managedCluster managedcluster-${i}..."
    cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name:  managedcluster-${i}
spec:
  hubAcceptsClient: true
EOF

    # patch the managedcluster status -> clusterID
    uuid=$(printf "%08d-0000-0000-0000-%012d" "${cluster_id_prefix}" "${i}") # "00000000-0000-0000-0000-000000000000"
    kubectl patch ManagedCluster managedcluster-${i} --type=merge --subresource status --patch "status: {clusterClaims: [{name: id.k8s.io, value: ${uuid}}]}"

    # patch the managedcluster status -> conditions
    kubectl patch ManagedCluster managedcluster-${i} --type=merge --subresource status --patch 'status: {conditions: [{lastTransitionTime: 2023-08-23T05:25:09Z, message: "Accepted by hub cluster admin", reason: "HubClusterAdminAccepted", status: "True", type: "HubAcceptedManagedCluster"}, {lastTransitionTime: 2023-08-23T05:25:10Z, message: "Managed cluster joined", reason: "ManagedClusterJoined", status: "True", type: "ManagedClusterJoined"}, {lastTransitionTime: 2023-08-23T05:25:10Z, message: "ManagedClusterAvailable", reason: "ManagedClusterAvailable", status: "True", type: "ManagedClusterConditionAvailable"}]}'
done