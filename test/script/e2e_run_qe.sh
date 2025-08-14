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

if [[ $PULL_BASE_REF =~ 'release-' ]]; then
  branchVersion=$(echo -n $branchName | awk -F '-' '{print $2}')
  imageTag="v$branchVersion"
else
  imageTag="latest"
fi

docker pull quay.io/hchenxa/acmqe-hoh-e2e:$imageTag
docker run --name qe-test -d quay.io/hchenxa/acmqe-hoh-e2e:$imageTag  tail -f /dev/null
docker cp qe-test:/e2e.test ./

SERVICE_TYPE=NODE_PORT KUBECONFIG=$GH_KUBECONFIG SPOKE_KUBECONFIG=$MH1_KUBECONFIG ./e2e.test --ginkgo.fail-fast --ginkgo.vv --ginkgo.label-filter='e2e && !migration'
