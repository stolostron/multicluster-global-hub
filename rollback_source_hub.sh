#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Source Hub Rollback Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Step 1: Get migration name
read -p "Enter the Migration CR name: " MIGRATION_NAME
if [ -z "$MIGRATION_NAME" ]; then
    echo -e "${RED}Error: Migration name cannot be empty${NC}"
    exit 1
fi

# Step 2: Get target hub name (manual input only)
echo -e "\n${YELLOW}Please enter the target hub name.${NC}"
read -p "Target Hub Name: " TARGET_HUB_NAME
if [ -z "$TARGET_HUB_NAME" ]; then
    echo -e "${RED}Error: Target hub name cannot be empty${NC}"
    exit 1
fi
echo -e "${GREEN}Using Target Hub Name: ${TARGET_HUB_NAME}${NC}"

# Step 3: Get list of clusters to rollback (manual input only)
echo -e "\n${YELLOW}Please enter the cluster names to rollback.${NC}"
echo -e "${YELLOW}Tip: You can get the failed cluster list from target hub's migration ConfigMap:${NC}"
echo -e "${YELLOW}  kubectl get configmap ${MIGRATION_NAME} -n multicluster-global-hub -o jsonpath='{.data.failure}'${NC}"
echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
read -p "Enter cluster names (comma-separated) or press Enter to rollback all migrating clusters: " CLUSTERS_TO_ROLLBACK

# Convert comma-separated list to array
if [ -n "$CLUSTERS_TO_ROLLBACK" ]; then
    IFS=',' read -ra CLUSTER_ARRAY <<< "$CLUSTERS_TO_ROLLBACK"
else
    # Get all clusters with migration annotation
    echo -e "${YELLOW}Fetching all clusters with migration annotation...${NC}"
    mapfile -t CLUSTER_ARRAY < <(kubectl get managedcluster -o jsonpath='{range .items[?(@.metadata.annotations.global-hub\.open-cluster-management\.io/migrating)]}{.metadata.name}{"\n"}{end}')
    if [ ${#CLUSTER_ARRAY[@]} -eq 0 ]; then
        echo -e "${YELLOW}No clusters found with migration annotation${NC}"
    fi
fi

echo -e "\n${GREEN}Clusters to rollback:${NC}"
printf '%s\n' "${CLUSTER_ARRAY[@]}"
echo ""
read -p "Proceed with rollback? (yes/no): " PROCEED
if [ "$PROCEED" != "yes" ]; then
    echo -e "${RED}Rollback cancelled by user${NC}"
    exit 1
fi

# Step 5: Restore HubAcceptsClient for failed clusters
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 1: Restoring HubAcceptsClient to true for clusters...${NC}"
    for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
        if [ -n "$CLUSTER" ]; then
            echo -e "  Restoring connectivity for cluster: ${CLUSTER}"
            kubectl patch managedcluster "$CLUSTER" --type=json \
                -p='[{"op": "replace", "path": "/spec/hubAcceptsClient", "value": true}]' 2>/dev/null || \
                echo -e "    ${YELLOW}Warning: Could not patch $CLUSTER (may not exist)${NC}"
        fi
    done
    echo -e "${GREEN}✓ HubAcceptsClient restored${NC}"
fi

# Step 6: Remove migration annotations from ManagedClusters
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 2: Removing migration annotations from ManagedClusters...${NC}"
    for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
        if [ -n "$CLUSTER" ]; then
            echo -e "  Processing cluster: ${CLUSTER}"
            kubectl annotate managedcluster "$CLUSTER" \
                global-hub.open-cluster-management.io/migrating- \
                agent.open-cluster-management.io/klusterlet-config- 2>/dev/null || \
                echo -e "    ${YELLOW}Warning: Could not remove annotations from $CLUSTER${NC}"
        fi
    done
    echo -e "${GREEN}✓ Migration annotations removed${NC}"
fi

# Step 7: Delete bootstrap secret
echo -e "\n${YELLOW}Step 3: Deleting bootstrap secret...${NC}"
BOOTSTRAP_SECRET="bootstrap-${TARGET_HUB_NAME}"
kubectl delete secret "$BOOTSTRAP_SECRET" -n multicluster-engine 2>/dev/null && \
    echo -e "${GREEN}✓ Bootstrap secret deleted: ${BOOTSTRAP_SECRET}${NC}" || \
    echo -e "${YELLOW}⚠ Bootstrap secret not found or already deleted: ${BOOTSTRAP_SECRET}${NC}"

# Step 8: Delete KlusterletConfig
echo -e "\n${YELLOW}Step 4: Deleting KlusterletConfig...${NC}"
KLUSTERLET_CONFIG="migration-${TARGET_HUB_NAME}"
kubectl delete klusterletconfig "$KLUSTERLET_CONFIG" 2>/dev/null && \
    echo -e "${GREEN}✓ KlusterletConfig deleted: ${KLUSTERLET_CONFIG}${NC}" || \
    echo -e "${YELLOW}⚠ KlusterletConfig not found or already deleted: ${KLUSTERLET_CONFIG}${NC}"

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}Source Hub Rollback Completed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "\nNext steps:"
echo -e "1. Run the target hub rollback script if needed"
echo -e "2. Verify that clusters have reconnected to the source hub"
echo -e "3. Check managedcluster status: ${YELLOW}kubectl get managedcluster${NC}"
