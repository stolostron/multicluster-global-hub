#!/bin/bash
#
# PREREQUISITE:
#  Docker KinD kubectl clusteradm 
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM

set -e

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LEAF_HUB_LOG=${LEAF_HUB_LOG:-$CONFIG_DIR/leafhub_setup.log}
HUB_CLUSTER_NUM=$1
MANAGED_CLUSTER_NUM=$2

source ${CURRENT_DIR}/common.sh

checkDir ${CONFIG_DIR}
checkKind
checkKubectl
checkClusteradm

KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/kubeconfig}
sleep 1 &
hover $! "  Leaf Hub: export KUBECONFIG=${KUBECONFIG}" 

# initCluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initKinDCluster "hub${i}" >> "$LEAF_HUB_LOG" 2>&1 &
  hover $! "  Create KinD Cluster hub${i}" &
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initKinDCluster "hub${i}-cluster${j}" >> "$LEAF_HUB_LOG" 2>&1 &
    hover $! "  Create KinD Cluster hub${i}-cluster${j}" &
  done
  wait

  # enableRouter
  enableRouter "kind-hub${i}" 2>&1 >> "$LEAF_HUB_LOG" &
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    enableRouter "kind-hub${i}-cluster${j}" 2>&1 >> "$LEAF_HUB_LOG" &
  done

  # apply multiclusterhubs.crd.yaml
  kubectl --context kind-hub${i} apply -f ${CURRENT_DIR}/../pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml
  # init ocm
  initHub "kind-hub${i}" "${CONFIG_DIR}/kind-hub${i}" 2>&1 >> "$LEAF_HUB_LOG" &
  hover $! "  OCM init hub kind-hub${i}"
  initManaged "kind-hub${i}" "kind-hub${i}-cluster" ${MANAGED_CLUSTER_NUM} 2>&1 >> "$LEAF_HUB_LOG"
  hover $! "  OCM join managed kind-hub${i}-cluster"
  wait

  HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig-hub-hub${i}
  kind get kubeconfig --name "hub${i}" --internal > "$HUB_KUBECONFIG"
  initPolicy "kind-hub${i}" "kind-hub${i}-cluster" ${MANAGED_CLUSTER_NUM} "$HUB_KUBECONFIG" 2>&1 >> "$LEAF_HUB_LOG" &
  initApp "kind-hub${i}" "kind-hub${i}-cluster" ${MANAGED_CLUSTER_NUM} 2>&1 >> "$LEAF_HUB_LOG" &  
done

# create cluster claim on managedcluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    cat <<EOF | kubectl --context kind-hub${i}-cluster${j} apply -f -
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: ClusterClaim
metadata:
  labels:
    open-cluster-management.io/hub-managed: ""
    velero.io/exclude-from-backup: "true"
  name: id.k8s.io
spec:
  value: $(uuidgen)
EOF
  done
done