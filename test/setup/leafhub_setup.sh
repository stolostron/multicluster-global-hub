#!/bin/bash
#
# PREREQUISITE:
#  Docker KinD kubectl clusteradm 
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM

HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-1}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-2}

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${LOG:-$CONFIG_DIR/leafhub_setup.log}

source ${CURRENT_DIR}/common.sh

checkDir ${CONFIG_DIR}
checkKind
checkKubectl
checkClusteradm

KUBECONFIG=${KUBECONFIG:-CONFIG_DIR/kubeconfig}
sleep 1 &
hover $! "  Leaf Hub: export KUBECONFIG=${KUBECONFIG}" 

# initCluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initKinDCluster "hub${i}" >> "${LOG}" 2>&1 & 
  hover $! "  Create KinD Cluster hub${i}" 
  enableRouter "kind-hub${i}" >> "$LOG" 2>&1
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initKinDCluster "hub${i}-cluster${j}" >> "${LOG}" 2>&1 & 
    hover $! "  Create KinD Cluster hub${i}-cluster${j}" 
    enableRouter "kind-hub${i}-cluster${j}" >> "$LOG" 2>&1
  done
done

# init ocm
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initHub "kind-hub${i}" "${CONFIG_DIR}/kind-hub${i}" >> "${LOG}" 2>&1 &
  hover $! "  OCM init hub kind-hub${i}" 
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initManaged "kind-hub${i}" "kind-hub${i}-cluster${j}" "${CONFIG_DIR}/kind-hub${i}" >> "${LOG}" 2>&1 &
    hover $! "  OCM join managed kind-hub${i}-cluster${j}" 
  done
done

# init app
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initApp "kind-hub${i}" "kind-hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "  Application hub${i}-cluster${j}" 
  done
done

# init policy
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig_kind-hub${i}
  kind get kubeconfig --name "hub${i}" --internal > "$HUB_KUBECONFIG"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initPolicy "kind-hub${i}" "kind-hub${i}-cluster${j}" "$HUB_KUBECONFIG" >> "${LOG}" 2>&1 &
    hover $! "  Policy hub${i}-cluster${j}" 
  done
done
