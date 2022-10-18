#!/bin/bash
#
# PREREQUISITE:
#  Docker KinD kubectl clusteradm 
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM

set -e

HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-1}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-2}

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LEAF_HUB_LOG=${LEAF_HUB_LOG:-$CONFIG_DIR/leafhub_setup.log}

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
  hover $! "  Create KinD Cluster hub${i}" 
  enableRouter "kind-hub${i}" 2>&1 >> "$LEAF_HUB_LOG"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initKinDCluster "hub${i}-cluster${j}" >> "$LEAF_HUB_LOG" 2>&1 & 
    hover $! "  Create KinD Cluster hub${i}-cluster${j}" 
    enableRouter "kind-hub${i}-cluster${j}" 2>&1 >> "$LEAF_HUB_LOG"
  done
done

# init ocm
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initHub "kind-hub${i}" "${CONFIG_DIR}/kind-hub${i}" 2>&1 >> "$LEAF_HUB_LOG" &
  hover $! "  OCM init hub kind-hub${i}" 
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initManaged "kind-hub${i}" "kind-hub${i}-cluster${j}" 2>&1 >> "$LEAF_HUB_LOG" &
    hover $! "  OCM join managed kind-hub${i}-cluster${j}" 
    checkManagedCluster "kind-hub${i}" "kind-hub${i}-cluster${j}" 2>&1 >> "$LEAF_HUB_LOG"
  done
done

# init app
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initApp "kind-hub${i}" "kind-hub${i}-cluster${j}" 2>&1 >> "$LEAF_HUB_LOG" &
    hover $! "  Application hub${i}-cluster${j}" 
  done
done

# init policy
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig-hub-hub${i}
  kind get kubeconfig --name "hub${i}" --internal > "$HUB_KUBECONFIG"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initPolicy "kind-hub${i}" "kind-hub${i}-cluster${j}" "$HUB_KUBECONFIG" 2>&1 >> "$LEAF_HUB_LOG" &
    hover $! "  Policy hub${i}-cluster${j}" 
  done
done
