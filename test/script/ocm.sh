#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit; pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

# setup kubeconfig
KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}
HUB_INIT=${HUB_INIT:-true}
POLICY_INIT=${POLICY_INIT:-true}

hub="$1"
spoken="$2"
start_time=$(date +%s)

echo -e "\r${BOLD_GREEN}[ START - $(date +"%T") ] $hub : $spoken $NC"
set +e

# init clusters
if [ "$HUB_INIT" = true ]; then
  kind_cluster "$hub" || {
    echo -e "${RED}Failed to create hub cluster: $hub${NC}"
    exit 1
  }
  init_hub "$hub" || {
    echo -e "${RED}Failed to initialize hub: $hub${NC}"
    exit 1
  }
fi

kind_cluster "$spoken" || {
  echo -e "${RED}Failed to create spoken cluster: $spoken${NC}"
  exit 1
}

# Verify contexts exist before proceeding
max_retries=30
retry_count=0
while [ $retry_count -lt $max_retries ]; do
  if kubectl config get-contexts "$hub" >/dev/null 2>&1; then
    break
  fi
  echo -e "${YELLOW}Waiting for context $hub to be available (attempt $((retry_count + 1))/$max_retries)...${NC}"
  sleep 1
  retry_count=$((retry_count + 1))
done

if [ $retry_count -eq $max_retries ]; then
  echo -e "${RED}Context $hub not found after $max_retries attempts${NC}"
  echo -e "${RED}Available contexts:${NC}"
  kubectl config get-contexts || true
  exit 1
fi

retry_count=0
while [ $retry_count -lt $max_retries ]; do
  if kubectl config get-contexts "$spoken" >/dev/null 2>&1; then
    break
  fi
  echo -e "${YELLOW}Waiting for context $spoken to be available (attempt $((retry_count + 1))/$max_retries)...${NC}"
  sleep 1
  retry_count=$((retry_count + 1))
done

if [ $retry_count -eq $max_retries ]; then
  echo -e "${RED}Context $spoken not found after $max_retries attempts${NC}"
  echo -e "${RED}Available contexts:${NC}"
  kubectl config get-contexts || true
  exit 1
fi

install_crds "$hub"  # router, mch(not needed for the managed clusters)
install_crds "$spoken"

retry "(join_cluster $hub $spoken) && kubectl get mcl $spoken --context $hub" 5

# async
if [ "$POLICY_INIT" = true ]; then
  # init policy for managed hub clusters only
  init_policy "$hub" "$spoken"
fi

enable_cluster "$hub" "$spoken" 

wait_ocm "$hub" "$spoken"
if [ "$POLICY_INIT" = true ]; then
  wait_policy "$hub" "$spoken"
fi

echo -e "\r${BOLD_GREEN}[ END - $(date +"%T") ] $hub : $spoken ${NC} $(($(date +%s) - start_time)) seconds"