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

docker pull quay.io/hchenxa/acmqe-hoh-e2e
docker run --name qe-test -d quay.io/hchenxa/acmqe-hoh-e2e  tail -f /dev/null
docker cp qe-test:/e2e.test ./

SERVICE_TYPE=NODE_PORT KUBECONFIG=$GH_KUBECONFIG SPOKE_KUBECONFIG=$MH1_KUBECONFIG ./e2e.test --ginkgo.v --ginkgo.label-filter='e2e && !migration'
