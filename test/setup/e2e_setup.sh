#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
export LOG=${LOG:-$CONFIG_DIR/e2e_setup.log}
export TAG=${TAG:-"latest"}
export LOG_MODE=${LOG_MODE:-INFO}

source ${CURRENT_DIR}/common.sh
checkDir ${CONFIG_DIR}

LEAF_HUB_NAME="hub1"
HUB_OF_HUB_NAME="hub-of-hubs"
CTX_HUB="microshift" #"kind-hub-of-hubs"
CTX_MANAGED="kind-hub1"

# setup kubeconfig
export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/kubeconfig}
echo "export KUBECONFIG=$KUBECONFIG" > $LOG
sleep 1 &
hover $! "KUBECONFIG=${KUBECONFIG}"

# init hoh 
source ${CURRENT_DIR}/microshift/microshift_setup.sh "$HUB_OF_HUB_NAME" 2>&1 >> $LOG &
hover $! "1 Prepare top hub cluster $HUB_OF_HUB_NAME"

# isolate the hub kubeconfig
HOH_KUBECONFIG=${CONFIG_DIR}/kubeconfig-hub-${CTX_HUB} # kind get kubeconfig --name "$HUB_OF_HUB_NAME" --internal > "$HOH_KUBECONFIG"
kubectl config view --context=${CTX_HUB} --minify --flatten > ${HOH_KUBECONFIG}

# enable olm
enableOLM $CTX_HUB 2>&1 >> $LOG &
hover $! "  Enable OLM for $CTX_HUB"

# install some component in microshift in detached mode
bash ${CURRENT_DIR}/hoh/postgres_setup.sh $HOH_KUBECONFIG 2>&1 >> $LOG &
bash ${CURRENT_DIR}/hoh/kafka_setup.sh $HOH_KUBECONFIG 2>&1 >> $LOG &
initHub $CTX_HUB 2>&1 >> $LOG &

# init leafhub 
sleep 1 &
hover $! "2 Prepare leaf hub cluster $LEAF_HUB_NAME"
source ${CURRENT_DIR}/leafhub_setup.sh 

# joining lh to hoh
initHub $CTX_HUB 2>&1 >> $LOG &
hover $! "3 Init HoH OCM $HUB_OF_HUB_NAME" 

# check connection
connectMicroshift "${LEAF_HUB_NAME}-control-plane" "${HUB_OF_HUB_NAME}" 2>&1 >> $LOG &
hover $! "  Check connection: $LEAF_HUB_NAME -> $HUB_OF_HUB_NAME" 

initManaged $CTX_HUB $CTX_MANAGED 2>&1 >> $LOG &
hover $! "  Joining $CTX_HUB - $CTX_MANAGED" 
checkManagedCluster $CTX_HUB $CTX_MANAGED 2>&1 >> $LOG

initApp $CTX_HUB $CTX_MANAGED 2>&1 >> $LOG &
hover $! "  Enable application $CTX_HUB - $CTX_MANAGED" 

initPolicy $CTX_HUB $CTX_MANAGED $HOH_KUBECONFIG 2>&1 >> $LOG &
hover $! "  Enable Policy $CTX_HUB - $CTX_MANAGED" 

kubectl config use-context $CTX_HUB >> $LOG

# wait kafka to be ready
waitKafkaToBeReady

# wait postgres to be ready
waitPostgresToBeReady

# deploy hoh
# need the following labels to enable deploying agent in leaf hub cluster
kubectl label managedcluster kind-$LEAF_HUB_NAME vendor=OpenShift --overwrite 2>&1 >> $LOG 
source ${CURRENT_DIR}/hoh/hoh_setup.sh >> $LOG 2>&1 &
hover $! "6 Deploy hub-of-hubs with $TAG" 

export KUBECONFIG=$KUBECONFIG
printf "%s\033[0;32m%s\n\033[0m" "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"