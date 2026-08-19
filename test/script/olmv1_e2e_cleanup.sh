#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

cluster_name="global-hub-olmv1"

echo -e "${YELLOW}=== OLMv1 E2E Cleanup ===${NC}"

# Delete KinD cluster
echo -e "${YELLOW}Deleting KinD cluster: ${cluster_name}${NC}"
kind delete cluster --name "$cluster_name" 2>/dev/null || true

# Stop local registry
echo -e "${YELLOW}Stopping local registry${NC}"
stop_local_registry

# Clean up kubeconfig
rm -f "${CONFIG_DIR}/${cluster_name}" 2>/dev/null || true

echo -e "${GREEN}=== Cleanup complete ===${NC}"
