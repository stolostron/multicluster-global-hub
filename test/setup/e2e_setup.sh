#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
export LOG=${LOG:-$CONFIG_DIR/e2e_setup.log}
export TAG=${TAG:-"latest"}
export LOG_MODE=${LOG_MODE:-INFO}

source ${CURRENT_DIR}/common.sh
checkDir ${CONFIG_DIR}

LEAF_HUB_NAME="hub"
HUB_OF_HUB_NAME="hub-of-hubs"
CTX_HUB="microshift" #"kind-hub-of-hubs"
CTX_MANAGED="kind-hub"
HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-2}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-1}

# setup kubeconfig
export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/kubeconfig}
echo "export KUBECONFIG=$KUBECONFIG" > $LOG
sleep 1 &
hover $! "KUBECONFIG=${KUBECONFIG}"

# create hub-of-hubs cluster 
source ${CURRENT_DIR}/microshift/microshift_setup.sh "$HUB_OF_HUB_NAME" 2>&1 >> $LOG &
# create leafhub clusters
source ${CURRENT_DIR}/leafhub_setup.sh "$HUB_CLUSTER_NUM" "$MANAGED_CLUSTER_NUM" 2>&1 >> $LOG &

wait

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

# joining lh to hoh
initHub $CTX_HUB 2>&1 >> $LOG &
hover $! "3 Init HoH OCM $HUB_OF_HUB_NAME" 

# check connection
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
    connectMicroshift "${LEAF_HUB_NAME}${i}" "${HUB_OF_HUB_NAME}" 2>&1 >> $LOG &
    hover $! "  Check connection: $LEAF_HUB_NAME$i -> $HUB_OF_HUB_NAME" 

    initManaged $CTX_HUB $CTX_MANAGED$i 2>&1 >> $LOG &
    hover $! "  Joining $CTX_HUB - $CTX_MANAGED$i" 
    checkManagedCluster $CTX_HUB $CTX_MANAGED$i 2>&1 >> $LOG

    initApp $CTX_HUB $CTX_MANAGED$i 2>&1 >> $LOG &
    hover $! "  Enable application $CTX_HUB - $CTX_MANAGED$i" 

    initPolicy $CTX_HUB $CTX_MANAGED$i $HOH_KUBECONFIG 2>&1 >> $LOG &
    hover $! "  Enable Policy $CTX_HUB - $CTX_MANAGED$i" 

    kubectl config use-context $CTX_HUB >> $LOG
done

# wait kafka to be ready
waitAppear "kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true" 1200
waitAppear "kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' --ignore-not-found=true" 1200

# wait postgres to be ready
waitAppear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"

#deploy hoh
#need the following labels to enable deploying agent in leaf hub cluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
    kubectl label managedcluster kind-$LEAF_HUB_NAME$i vendor=OpenShift --overwrite 2>&1 >> $LOG 
done
source ${CURRENT_DIR}/hoh/hoh_setup.sh >> $LOG 2>&1 &
hover $! "6 Deploy hub-of-hubs with $TAG" 

export KUBECONFIG=$KUBECONFIG
printf "%s\033[0;32m%s\n\033[0m" "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"
