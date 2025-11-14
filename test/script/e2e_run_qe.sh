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
  branchVersion=$(echo -n $PULL_BASE_REF | awk -F '-' '{print $2}')
  imageTag="v$branchVersion"
else
  imageTag="latest"
fi

docker pull "quay.io/hchenxa/acmqe-hoh-e2e:$imageTag"

echo "GH_KUBECONFIG: $GH_KUBECONFIG"
echo "MH1_KUBECONFIG: $MH1_KUBECONFIG"

# Find the Docker network used by kind cluster
KIND_NETWORK=$(docker network ls --format "{{.Name}}" | grep "kind" | head -1)

if [[ -z "$KIND_NETWORK" ]]; then
  echo "Warning: No kind network found, trying to detect from running containers..."
  # Try to get network name from kind container
  KIND_CONTAINER=$(docker ps --format "{{.Names}}" | grep "kind.*control-plane" | head -1)
  if [[ -n "$KIND_CONTAINER" ]]; then
    KIND_NETWORK=$(docker inspect "$KIND_CONTAINER" --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}}{{end}}' | head -1)
    echo "Detected kind network from container $KIND_CONTAINER: $KIND_NETWORK"
  else
    echo "Error: Cannot find kind network or containers"
    echo "Available networks:"
    docker network ls
    echo "Available containers:"
    docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
    exit 1
  fi
else
  echo "Found kind network: $KIND_NETWORK"
fi

# Show container information in kind network
echo "=== Kind network containers ==="
docker network inspect "$KIND_NETWORK" --format '{{range .Containers}}{{.Name}}: {{.IPv4Address}}{{println}}{{end}}'

# Use original kubeconfig (no modification needed)
echo "Using original kubeconfigs - no modification needed when on same network"

# Run e2e tests, join kind network
echo "Running Docker container on kind network: $KIND_NETWORK"
docker run --rm \
  --network "$KIND_NETWORK" \
  -v "$GH_KUBECONFIG:/kubeconfig/global-hub:ro" \
  -v "$MH1_KUBECONFIG:/kubeconfig/hub1:ro" \
  -e SERVICE_TYPE=NODE_PORT \
  -e KUBECONFIG=/kubeconfig/global-hub \
  -e SPOKE_KUBECONFIG=/kubeconfig/hub1 \
  "quay.io/hchenxa/acmqe-hoh-e2e:$imageTag" \
  /e2e.test --ginkgo.fail-fast --ginkgo.vv --ginkgo.label-filter='e2e && !migration'