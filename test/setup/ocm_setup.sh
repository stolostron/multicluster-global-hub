#!/bin/bash
#
# PREREQUISITE:
#  Docker
#  KinD v0.12.0  https://kind.sigs.k8s.io/docs/user/quick-start/ 
#  kubectl client
#  clusteradm client: MacOS M1 needs to install clusteradm in advance
# PARAMETERS:
#  ENV HUB_CLUSTER_NUM
#  ENV MANAGED_CLUSTER_NUM
# NOTE:
#  the ocm_setup.sh script must be located under the same directory as the ocm_clean.sh 
#

HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-1}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-2}

ROOT_DIR=$(cd "$(dirname "$0")/../.." ; pwd -P)
CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${CONFIG_DIR}/ocm_setup.log
PID=${CONFIG_DIR}/pid

source ${CURRENT_DIR}/common.sh

checkDir ${CONFIG_DIR}
checkKind
checkKubectl
checkClusteradm

KUBECONFIG=${KUBECONFIG:-CONFIG_DIR/kubeconfig}
sleep 1 &
hover $! "export KUBECONFIG=${KUBECONFIG}" "${PID}"

# initCluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initKinDCluster "hub${i}" >> "${LOG}" 2>&1 & 
  hover $! "Create KinD Cluster hub${i}" "${PID}"
  enableRouter "kind-hub${i}" "${CURRENT_DIR}/router/" >> "$LOG" 2>&1
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initKinDCluster "hub${i}-cluster${j}" >> "${LOG}" 2>&1 & 
    hover $! "Create KinD Cluster hub${i}-cluster${j}" "${PID}"
    enableRouter "kind-hub${i}-cluster${j}" "${CURRENT_DIR}/router/" >> "$LOG" 2>&1
  done
done

# init ocm
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  initHub "kind-hub${i}" "${CONFIG_DIR}/kind-hub${i}" >> "${LOG}" 2>&1 &
  hover $! "OCM init hub kind-hub${i}" "${PID}"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initManaged "kind-hub${i}" "kind-hub${i}-cluster${j}" "${CONFIG_DIR}/kind-hub${i}" >> "${LOG}" 2>&1 &
    hover $! "OCM join managed kind-hub${i}-cluster${j}" "${PID}"
  done
done

# init app
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initApp "kind-hub${i}" "kind-hub${i}-cluster${j}" >> "${LOG}" 2>&1 &
    hover $! "Application hub${i}-cluster${j}" "${PID}"
  done
done

# init policy
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig_kind-hub${i}
  kind get kubeconfig --name "hub${i}" --internal > "$HUB_KUBECONFIG"
  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    initPolicy "kind-hub${i}" "kind-hub${i}-cluster${j}" "$HUB_KUBECONFIG" >> "${LOG}" 2>&1 &
    hover $! "Policy hub${i}-cluster${j}" "${PID}"
  done
done

printf "%s\033[0;32m%s\n\033[0m " "[Access the Clusters]: " "export KUBECONFIG=${KUBECONFIG}"