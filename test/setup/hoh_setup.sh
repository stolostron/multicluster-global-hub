#!/bin/bash

ROOT_DIR=$(cd "$(dirname "$0")/../.." ; pwd -P)
CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
LOG=${CONFIG_DIR}/hoh_setup.log
PID=${CONFIG_DIR}/pid

source ${CURRENT_DIR}/common.sh

checkDir ${CONFIG_DIR}
checkKind
checkKubectl
checkClusteradm

LEAF_HUB_NAME="hub1"
HUB_OF_HUB_NAME="hub-of-hubs"

CTX_HUB="kind-hub-of-hubs"
CTX_MANAGED="kind-hub1"

# kubeconfig
KUBECONFIG=${CONFIG_DIR}/kubeconfig
sleep 1 &
hover $! "KUBECONFIG=${KUBECONFIG}" "$PID"

# init hub-of-hubs cluster
initKinDCluster "$HUB_OF_HUB_NAME" >> "${LOG}" 2>&1 & 
hover $! "HoH Create KinD Cluster $HUB_OF_HUB_NAME" "$PID"
enableRouter "kind-$HUB_OF_HUB_NAME" >> "$LOG" 2>&1
enableServiceCA "kind-$HUB_OF_HUB_NAME" "${CURRENT_DIR}/service-ca/" >> "$LOG" 2>&1

# init leaf hub cluster
bash ${CURRENT_DIR}/ocm_setup.sh >> $LOG 2>&1 &
hover $! "HoH Create Leaf Hub Cluster $LEAF_HUB_NAME" "$PID"

# deloy ocm on hub of hubs 
initHub $CTX_HUB "${CONFIG_DIR}/${CTX_HUB}" >> $LOG 2>&1 &
hover $! "HoH OCM Hub $CTX_HUB" "$PID"

initManaged $CTX_HUB $CTX_MANAGED "${CONFIG_DIR}/${CTX_HUB}" >> $LOG 2>&1 &
hover $! "HoH OCM Managed $CTX_HUB - $CTX_MANAGED" "$PID"

# add application to hub of hubs
initApp $CTX_HUB $CTX_MANAGED >> "${LOG}" 2>&1 &
hover $! "HoH Application $CTX_HUB - $CTX_MANAGED" "$PID"

# add policy to hub of hubs
HUB_KUBECONFIG=${CONFIG_DIR}/kubeconfig_${CTX_HUB}
# kubectl config view --context=${hub} --minify --flatten > ${HUB_KUBECONFIG}
kind get kubeconfig --name "$HUB_OF_HUB_NAME" --internal > "$HUB_KUBECONFIG"
initPolicy $CTX_HUB $CTX_MANAGED $HUB_KUBECONFIG >> "${LOG}" 2>&1 &
hover $! "HoH Policy $CTX_HUB - $CTX_MANAGED" "$PID"

addOperatorLifecycleManager "$CTX_HUB" >> "${LOG}" 2>&1 &
hover $! "HoH OLM $CTX_HUB" "$PID"

printf "%s\033[0;32m%s\n\033[0m" "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"