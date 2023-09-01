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
HUB_OF_HUB_NAME="global-hub"
CTX_HUB="kind-global-hub"
CTX_MANAGED="kind-hub"
HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-2}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-1}

# setup kubeconfig
export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/kubeconfig}
echo "export KUBECONFIG=$KUBECONFIG" > $LOG
sleep 1 &
hover $! "KUBECONFIG=${KUBECONFIG}"
startTime_s=`date +%s`

# init hoh
initKinDCluster "$HUB_OF_HUB_NAME" >> $LOG 2>&1 &
hover $! "1 Prepare top hub cluster $HUB_OF_HUB_NAME"
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${HUB_OF_HUB_NAME}-control-plane)
hub_kubeconfig="${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}"
kubectl --kubeconfig $hub_kubeconfig config set-cluster kind-${HUB_OF_HUB_NAME} --server=https://$global_hub_node_ip:6443
HOH_KUBECONFIG=${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}
# enable  route
enableRouter $CTX_HUB 2>&1 >> $LOG &
# enable service CA
enableServiceCA $CTX_HUB ${HUB_OF_HUB_NAME} ${CURRENT_DIR} 2>&1 >> $LOG &

endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "Prepare top hub :$sumTime seconds"

# install some component in global hub in async mode
bash ${CURRENT_DIR}/hoh/postgres/postgres_setup.sh $HOH_KUBECONFIG 2>&1 >> $LOG &
bash ${CURRENT_DIR}/hoh/kafka/kafka_setup.sh $CTX_HUB 2>&1 >> $LOG &
initHub $CTX_HUB 2>&1 >> $LOG &
startTime_s=`date +%s`

# init leafhub
sleep 1 &
hover $! "2 Prepare leaf hub cluster $LEAF_HUB_NAME"
bash ${CURRENT_DIR}/leafhub_setup.sh "$HUB_CLUSTER_NUM" "$MANAGED_CLUSTER_NUM"
endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "Prepare leaf hub cluster :$sumTime seconds"

startTime_s=`date +%s`

# import managed hubs
initManaged $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} 2>&1 >> $LOG &
hover $! "  Joining $CTX_HUB"

initApp $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} 2>&1 >> $LOG &
initPolicy $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} $HOH_KUBECONFIG 2>&1 >> $LOG &

endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "check connection :$sumTime seconds"
startTime_s=`date +%s`
kubectl config use-context $CTX_HUB >> $LOG
# wait kafka to be ready
waitAppear "kubectl get pods -n kafka -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true" 1200
waitAppear "kubectl get kafka kafka-brokers-cluster -n kafka -o jsonpath='{.status.listeners[1].certificates[0]}' --ignore-not-found=true" 1200

# wait postgres to be ready
waitAppear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"

#need the following labels to enable deploying agent in leaf hub cluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
    kubectl label managedcluster kind-$LEAF_HUB_NAME$i vendor=OpenShift --overwrite 2>&1 >> $LOG

    # Deprecated: Upgrade the leafhub environment to compatiable with the latest placement and placementbinding API
    # 1. The clusteradm install the placement by the registration-operator
    # https://github.com/open-cluster-management-io/registration-operator/pull/360(sync api to registration-operator)
    kubectl --context kind-$LEAF_HUB_NAME$i apply -f https://raw.githubusercontent.com/open-cluster-management-io/api/main/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placements.crd.yaml
    # 2. The placementbinding is installed by governance-policy-propagator, while the API was updated by this: 
    # https://github.com/open-cluster-management-io/governance-policy-propagator/pull/110
    kubectl --context kind-$LEAF_HUB_NAME$i apply -f https://raw.githubusercontent.com/open-cluster-management-io/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_placementbindings.yaml
done
export KUBECONFIG=$KUBECONFIG
# TODO: think about readinessCheck

printf "%s\033[0;32m%s\n\033[0m" "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"