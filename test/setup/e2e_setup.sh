#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
CONFIG_DIR=${CURRENT_DIR}/config
export LOG=${LOG:-$CONFIG_DIR/e2e_setup.log}
export TAG=${TAG:-"latest"}
export LOG_MODE=${LOG_MODE:-INFO}

source ${CURRENT_DIR}/common.sh
check_dir ${CONFIG_DIR}

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
kind_cluster "$HUB_OF_HUB_NAME" >> $LOG 2>&1 &
hover $! "1 Prepare top hub cluster $HUB_OF_HUB_NAME"
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${HUB_OF_HUB_NAME}-control-plane)
hub_kubeconfig="${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}"
kubectl --kubeconfig $hub_kubeconfig config set-cluster kind-${HUB_OF_HUB_NAME} --server=https://$global_hub_node_ip:6443
HOH_KUBECONFIG=${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}
# enable  route
enable_router $CTX_HUB 2>&1 >> $LOG &
# enable service CA
enable_service_ca $CTX_HUB ${HUB_OF_HUB_NAME} ${CURRENT_DIR} 2>&1 >> $LOG &

endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "Prepare top hub :$sumTime seconds"

# install some component in global hub in async mode
bash ${CURRENT_DIR}/hoh/postgres/postgres_setup.sh $HOH_KUBECONFIG 2>&1 >> $LOG &
bash ${CURRENT_DIR}/hoh/kafka/kafka_setup.sh $CTX_HUB 2>&1 >> $LOG &
init_hub $CTX_HUB 2>&1 >> $LOG &
startTime_s=`date +%s`

# apply multiclusterhubs.crd.yaml
kubectl --kubeconfig $hub_kubeconfig apply -f ${CURRENT_DIR}/../../pkg/testdata/crds/0000_01_operator.open-cluster-management.io_multiclusterhubs.crd.yaml

# init leafhub
sleep 1 &
hover $! "2 Prepare leaf hub cluster $LEAF_HUB_NAME"
bash ${CURRENT_DIR}/leafhub_setup.sh "$HUB_CLUSTER_NUM" "$MANAGED_CLUSTER_NUM"
endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "Prepare leaf hub cluster :$sumTime seconds"

startTime_s=`date +%s`

# import managed hubs
init_managed $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} 2>&1 >> $LOG &
hover $! "  Joining $CTX_HUB"

init_app $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} 2>&1 >> $LOG &
init_policy $CTX_HUB $CTX_MANAGED ${HUB_CLUSTER_NUM} $HOH_KUBECONFIG 2>&1 >> $LOG &

endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "check connection :$sumTime seconds"
startTime_s=`date +%s`
kubectl config use-context $CTX_HUB >> $LOG
# wait kafka to be ready
wait_appear "kubectl get pods -n multicluster-global-hub -l name=strimzi-cluster-operator --ignore-not-found | grep Running || true" 1200
wait_appear "kubectl get kafka kafka -n multicluster-global-hub -o jsonpath='{.status.listeners[1].certificates[0]}' --ignore-not-found=true" 1200

# wait postgres to be ready
wait_appear "kubectl get secret hoh-pguser-postgres -n hoh-postgres --ignore-not-found=true"

#need the following labels to enable deploying agent in leaf hub cluster
for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
    kubectl label managedcluster kind-$LEAF_HUB_NAME$i vendor=OpenShift --overwrite 2>&1 >> $LOG
    # add clusterclaim 
    cat <<EOF | kubectl --context kind-$LEAF_HUB_NAME$i apply -f -
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
export KUBECONFIG=$KUBECONFIG
# TODO: think about readinessCheck

printf "%s\033[0;32m%s\n\033[0m" "[Access the Clusters]: " "export KUBECONFIG=$KUBECONFIG"