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

# 获取Docker host IP（Docker bridge的gateway IP）
HOST_IP=$(docker network inspect bridge | grep -o '"Gateway": "[^"]*' | grep -o '[^"]*$' | head -1)
if [[ -z "$HOST_IP" ]]; then
  HOST_IP="172.17.0.1"  # 默认Docker bridge gateway
fi
echo "Docker Host IP: $HOST_IP"

# 函数：更新kubeconfig中的server地址，使其能从Docker容器访问
update_kubeconfig_for_docker() {
  local kubeconfig_path="$1"
  local temp_kubeconfig="$2"
  
  # 复制原始kubeconfig
  cp "$kubeconfig_path" "$temp_kubeconfig"
  
  # 获取原始的端口号
  local original_port=$(grep -o 'https://[^:]*:\([0-9]*\)' "$temp_kubeconfig" | grep -o '[0-9]*$' | head -1)
  
  if [[ -n "$original_port" ]]; then
    echo "Found kind cluster port: $original_port"
    
    # 将server地址替换为Docker host IP + 原端口
    sed -i "s|https://[^:]*:[0-9]*|https://$HOST_IP:$original_port|g" "$temp_kubeconfig"
    echo "Updated kubeconfig server to: https://$HOST_IP:$original_port"
  else
    echo "Warning: Could not find port in kubeconfig"
    # 备选方案：直接替换localhost和127.0.0.1
    sed -i "s|https://127\.0\.0\.1:|https://$HOST_IP:|g" "$temp_kubeconfig"
    sed -i "s|https://localhost:|https://$HOST_IP:|g" "$temp_kubeconfig"
  fi
}

# 创建临时目录存放修改后的kubeconfig
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

TEMP_GH_KUBECONFIG="$TEMP_DIR/global-hub"
TEMP_MH1_KUBECONFIG="$TEMP_DIR/hub1"

# 更新kubeconfig文件
update_kubeconfig_for_docker "$GH_KUBECONFIG" "$TEMP_GH_KUBECONFIG"
update_kubeconfig_for_docker "$MH1_KUBECONFIG" "$TEMP_MH1_KUBECONFIG"

# 验证更新后的配置
echo "=== Updated Global Hub Config ==="
grep "server:" "$TEMP_GH_KUBECONFIG" || echo "No server line found"
echo "=== Updated Hub1 Config ==="
grep "server:" "$TEMP_MH1_KUBECONFIG" || echo "No server line found"

# 运行e2e测试，使用host网络模式让容器能访问kind集群网络
echo "Running Docker container with host network to access kind cluster..."
docker run --rm \
  --network host \
  -v "$TEMP_GH_KUBECONFIG:/kubeconfig/global-hub:ro" \
  -v "$TEMP_MH1_KUBECONFIG:/kubeconfig/hub1:ro" \
  -e SERVICE_TYPE=NODE_PORT \
  -e KUBECONFIG=/kubeconfig/global-hub \
  -e SPOKE_KUBECONFIG=/kubeconfig/hub1 \
  "quay.io/hchenxa/acmqe-hoh-e2e:$imageTag" \
  /e2e.test --ginkgo.fail-fast --ginkgo.vv --ginkgo.label-filter='e2e && !migration'