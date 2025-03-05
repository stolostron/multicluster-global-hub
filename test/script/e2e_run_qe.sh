#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
source "$CURRENT_DIR/util.sh"

CONFIG_DIR=$CURRENT_DIR/config
GH_KUBECONFIG="${CONFIG_DIR}/global-hub"
MH1_KUBECONFIG="${CONFIG_DIR}/hub1"
export KUBECONFIG=$GH_KUBECONFIG
export GH_NAME="global-hub" # the KinD name


##Install globalhub
cd $CURRENT_DIR/../../operator
make deploy
cat <<EOF | kubectl apply --kubeconfig $GH_KUBECONFIG -f -
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
  annotations:
    mgh-skip-auth:  "true"
    mgh-scheduler-interval:   "minute"
    global-hub.open-cluster-management.io/catalog-source-name:      "operatorhubio-catalog"
    global-hub.open-cluster-management.io/catalog-source-namespace: "olm"
    global-hub.open-cluster-management.io/kafka-use-nodeport:       ""
spec:
  availabilityConfig: Basic
  enableMetrics: false
  dataLayer:
    postgres:
      retention: 18m
EOF

global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${GH_NAME}-control-plane)
export GLOBAL_HUB_NODE_IP=${global_hub_node_ip}

kubectl annotate mgh multiclusterglobalhub global-hub.open-cluster-management.io/kind-cluster-ip=$GLOBAL_HUB_NODE_IP --kubeconfig $GH_KUBECONFIG -n multicluster-global-hub

wait_cmd "kubectl get mgh --kubeconfig $GH_KUBECONFIG -n multicluster-global-hub | grep Running"

cd $CONFIG_DIR
docker pull quay.io/hchenxa/acmqe-hoh-e2e
docker run --name qe-test -d quay.io/hchenxa/acmqe-hoh-e2e  tail -f /dev/null
docker cp qe-test:/e2e.test ./

SERVICE_TYPE=NODE_PORT KUBECONFIG=$GH_KUBECONFIG SPOKE_KUBECONFIG=$MH1_KUBECONFIG ./e2e.test --ginkgo.v --ginkgo.label-filter='e2e'
