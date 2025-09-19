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

docker pull "quay.io/hchenxa/acmqe-hoh-e2e:$imageTag"

# Run e2e tests directly in container with mounted kubeconfig files
docker run --rm \
  -v "$GH_KUBECONFIG:/kubeconfig/global-hub:ro" \
  -v "$MH1_KUBECONFIG:/kubeconfig/hub1:ro" \
  -e SERVICE_TYPE=NODE_PORT \
  -e KUBECONFIG=/kubeconfig/global-hub \
  -e SPOKE_KUBECONFIG=/kubeconfig/hub1 \
  "quay.io/hchenxa/acmqe-hoh-e2e:$imageTag" \
  /e2e.test --ginkgo.fail-fast --ginkgo.vv --ginkgo.label-filter='e2e && !migration'
