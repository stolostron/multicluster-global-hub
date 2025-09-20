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

echo "GH_KUBECONFIG: $GH_KUBECONFIG"
echo "MH1_KUBECONFIG: $MH1_KUBECONFIG"

# 查找kind集群使用的Docker网络
KIND_NETWORK=$(docker network ls --format "{{.Name}}" | grep "kind" | head -1)

if [[ -z "$KIND_NETWORK" ]]; then
  echo "Warning: No kind network found, trying to detect from running containers..."
  # 尝试从kind容器获取网络名称
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

# 显示kind网络中的容器信息
echo "=== Kind network containers ==="
docker network inspect "$KIND_NETWORK" --format '{{range .Containers}}{{.Name}}: {{.IPv4Address}}{{println}}{{end}}'

# 使用原始kubeconfig（无需修改）
echo "Using original kubeconfigs - no modification needed when on same network"

# 运行e2e测试，加入kind网络
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