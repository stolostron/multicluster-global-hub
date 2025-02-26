#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

### This script is used to setup hub clusters for testing
### Usage: ./setup-cluster.sh <hub-cluster-number> <managed-cluster-number-on-each-hub> 
set -eo pipefail

# Check if the number of positional parameters is not equal to 1 or not equal to 2
if [ $# -ne 1 ] && [ $# -ne 2 ]; then
    echo "Usage: $0 <hub_start:hub_end> <cluster_start:cluster_end>"
    exit 1
fi

# Parse the parameter using the delimiter ":"
IFS=':' read -r hub_start hub_end <<< "$1"
IFS=':' read -r cluster_start cluster_end <<< "$2"

echo ">> Generate cluster ${cluster_start}~${cluster_end} on hub ${hub_start}~${hub_end}"

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../../.." ; pwd -P)"
export KUBECONFIG=${KUBECONFIG}
cluster_dir="${REPO_DIR}/doc/simulation/kubeconfig"
mkdir -p ${cluster_dir}

# creating the simulated hub clusters
for i in $(seq $hub_start $hub_end); do
    cluster=hub${i}
    if ! kind get clusters | grep -q "$cluster"; then
        echo ">> Creating KinD cluster: $cluster..."
        kubeconfig="${cluster_dir}/${cluster}"
        kind create cluster --kubeconfig $kubeconfig --name ${cluster} &
    else
        echo ">> KinD cluster '$cluster' already exists. Skipping..."
    fi
done

wait

# join the kind cluster to global hub
# 1. create cluster on global hub
for i in $(seq $hub_start $hub_end); do
    cluster="hub${i}"

    # on global hub: create cluster namespace, cluster, and KlusterletAddonConfig
    kubectl create ns $cluster --dry-run=client -oyaml | kubectl apply -f -
    cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name:  $cluster
spec:
  hubAcceptsClient: true
---
apiVersion: agent.open-cluster-management.io/v1
kind: KlusterletAddonConfig
metadata:
  name: $cluster
  namespace: $cluster
spec:
  clusterName: $cluster
  clusterNamespace: $cluster
  clusterLabels:
    name: $cluster
    cloud: auto-detect
    vendor: auto-detect
  applicationManager:
    enabled: true
  policyController:
    enabled: true
  searchCollector:
    enabled: true
  certPolicyController:
    enabled: true
  iamPolicyController:
    enabled: true
EOF

    klusterlet_crd=${cluster_dir}/${cluster}.import.crd.yaml
    imports=${cluster_dir}/${cluster}.import.yaml
    kubectl get secret "$cluster"-import -n "$cluster" -o jsonpath={.data.crds\\.yaml} | base64 --decode > $klusterlet_crd
    kubectl get secret "$cluster"-import -n "$cluster" -o jsonpath={.data.import\\.yaml} | base64 --decode > $imports

    # the KinD cluster:
    kubeconfig="${cluster_dir}/${cluster}"
    # 1. related namespace(kubectl version must be 1.24+)
    kubectl create ns application-system --dry-run=client -oyaml | kubectl --kubeconfig $kubeconfig apply -f -
    # apply the CRDs
    kubectl apply --server-side=true --validate=false --force-conflicts -f $REPO_DIR/test/manifest/crd --kubeconfig $kubeconfig

    # mock the NS/MultiClusterHub so that the agent can start
    kubectl create ns multicluster-global-hub --dry-run=client -oyaml | kubectl --kubeconfig $kubeconfig apply -f -
    kubectl apply -f $REPO_DIR/doc/simulation/setup/managed-clusters/multiclusterhub.yaml --kubeconfig $kubeconfig

    # apply the import resources
    kubectl apply --kubeconfig $kubeconfig -f $klusterlet_crd
    kubectl apply --kubeconfig $kubeconfig -f $imports
done

if [ $# -eq 1 ]; then
    echo "Access the clusters:"
    for i in $(seq $hub_start $hub_end); do
        hub_cluster=hub${i}
        kubeconfig="${cluster_dir}/${hub_cluster}"
        echo "export KUBECONFIG=${kubeconfig}"
    done
    exit 0
fi

function create_managed_cluster() {
    cluster_id=$1
    cluster_name=$2
    kubeconfig=$3
    cat <<EOF | kubectl apply --kubeconfig $kubeconfig -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: $cluster_name
spec:
  hubAcceptsClient: true
EOF

    # patch the managedcluster status -> clusterID
    kubectl --kubeconfig $kubeconfig patch ManagedCluster $cluster_name --type=merge --subresource status --patch "status: {clusterClaims: [{name: id.k8s.io, value: ${cluster_id}}]}"

    # patch the managedcluster status -> conditions
    kubectl --kubeconfig $kubeconfig patch ManagedCluster $cluster_name --type=merge --subresource status --patch 'status: {conditions: [{lastTransitionTime: 2023-08-23T05:25:09Z, message: "Accepted by hub cluster admin", reason: "HubClusterAdminAccepted", status: "True", type: "HubAcceptedManagedCluster"}, {lastTransitionTime: 2023-08-23T05:25:10Z, message: "Managed cluster joined", reason: "ManagedClusterJoined", status: "True", type: "ManagedClusterJoined"}, {lastTransitionTime: 2023-08-23T05:25:10Z, message: "ManagedClusterAvailable", reason: "ManagedClusterAvailable", status: "True", type: "ManagedClusterConditionAvailable"}]}'
}


for j in $(seq $cluster_start $cluster_end); do # for each managed cluster on the hub
  for i in $(seq $hub_start $hub_end); do # for each hub cluster
      hub_cluster=hub${i}
      cluster_name=managedcluster-${j}
      id=$(printf "%08d-0000-0000-0000-%012d" "${i}" "${j}") # "00000000-0000-0000-0000-000000000000"
      kubeconfig="${cluster_dir}/${hub_cluster}"
      echo ">> Creating ${hub_cluster}: ${cluster_name}..."
      create_managed_cluster ${id} ${cluster_name} ${kubeconfig} &
  done
  wait # waitting create the cluster on each hub
done

# printing the clusters
echo "Access the clusters:"
for i in $(seq $hub_start $hub_end); do
    hub_cluster=hub${i}
    kubeconfig="${cluster_dir}/${hub_cluster}"
    echo "export KUBECONFIG=${kubeconfig}"
done


