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

# Step 2: Get target hub name
echo -e "\n${YELLOW}Is the target hub name 'local-cluster'?${NC}"
read -p "Use 'local-cluster' as target hub name? (yes/no): " USE_LOCAL_CLUSTER
if [ "$USE_LOCAL_CLUSTER" = "yes" ] || [ "$USE_LOCAL_CLUSTER" = "y" ]; then
    TARGET_HUB_NAME="local-cluster"
else
    read -p "Enter the target hub name: " TARGET_HUB_NAME
    if [ -z "$TARGET_HUB_NAME" ]; then
        echo -e "${RED}Error: Target hub name cannot be empty${NC}"
        exit 1
    fi
fi
echo -e "${GREEN}Using Target Hub Name: ${TARGET_HUB_NAME}${NC}"

# Step 3: Get list of clusters to rollback (manual input only)
echo -e "\n${YELLOW}Please enter the cluster names to rollback.${NC}"
echo -e "${YELLOW}Tip: You can get the failed cluster list from target hub's migration ConfigMap:${NC}"
echo -e "${YELLOW}  kubectl get configmap ${MIGRATION_NAME} -n multicluster-global-hub -o jsonpath='{.data.failure}'${NC}"
echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
read -p "Enter cluster names (comma-separated) or press Enter to rollback all migrating clusters: " CLUSTERS_TO_ROLLBACK

# Convert comma-separated string to array
if [ -n "$CLUSTERS_TO_ROLLBACK" ]; then
    IFS=',' read -ra CLUSTER_ARRAY <<< "$CLUSTERS_TO_ROLLBACK"
else
    CLUSTER_ARRAY=()
fi

echo -e "\n${GREEN}Clusters to rollback:${NC}"
printf '%s\n' "${CLUSTER_ARRAY[@]}"
echo ""
read -p "Proceed with rollback? (yes/no): " PROCEED
if [ "$PROCEED" != "yes" ]; then
    echo -e "${RED}Rollback cancelled by user${NC}"
    exit 1
fi

# Step 1: Delete bootstrap secret (from rollbackInitializing)
echo -e "\n${YELLOW}Step 1: Deleting bootstrap secret...${NC}"
BOOTSTRAP_SECRET="bootstrap-${TARGET_HUB_NAME}"
kubectl delete secret "$BOOTSTRAP_SECRET" -n multicluster-engine 2>/dev/null && \
    echo -e "${GREEN}✓ Bootstrap secret deleted: ${BOOTSTRAP_SECRET}${NC}" || \
    echo -e "${YELLOW}⚠ Bootstrap secret not found or already deleted: ${BOOTSTRAP_SECRET}${NC}"

# Step 2: Delete KlusterletConfig (from rollbackInitializing)
echo -e "\n${YELLOW}Step 2: Deleting KlusterletConfig...${NC}"
KLUSTERLET_CONFIG="migration-${TARGET_HUB_NAME}"
kubectl delete klusterletconfig "$KLUSTERLET_CONFIG" 2>/dev/null && \
    echo -e "${GREEN}✓ KlusterletConfig deleted: ${KLUSTERLET_CONFIG}${NC}" || \
    echo -e "${YELLOW}⚠ KlusterletConfig not found or already deleted: ${KLUSTERLET_CONFIG}${NC}"

# Step 3: Remove migration annotations from ManagedClusters (from rollbackInitializing)
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 3: Removing migration annotations from ManagedClusters...${NC}"
    for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
        if [ -n "$CLUSTER" ]; then
            echo -e "  Processing cluster: ${CLUSTER}"
            kubectl annotate managedcluster "$CLUSTER" \
                global-hub.open-cluster-management.io/migrating- \
                agent.open-cluster-management.io/klusterlet-config- \
                import.open-cluster-management.io/disable-auto-import- 2>/dev/null || \
                echo -e "    ${YELLOW}Warning: Could not remove annotations from $CLUSTER${NC}"
        fi
    done
    echo -e "${GREEN}✓ Migration annotations removed${NC}"
fi

# Step 4: Delete managed-cluster-lease for each cluster (from rollbackRegistering)
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 4: Deleting managed-cluster-lease for clusters...${NC}"
    for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
        if [ -n "$CLUSTER" ]; then
            echo -e "  Deleting lease for cluster: ${CLUSTER}"
            kubectl delete lease managed-cluster-lease -n "$CLUSTER" 2>/dev/null && \
                echo -e "    ${GREEN}✓ Lease deleted for ${CLUSTER}${NC}" || \
                echo -e "    ${YELLOW}⚠ Lease not found or already deleted for ${CLUSTER}${NC}"
        fi
    done
    echo -e "${GREEN}✓ Managed cluster leases deleted${NC}"
fi

# Step 5: Restore HubAcceptsClient for clusters (from rollbackRegistering)
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 5: Restoring HubAcceptsClient to true for clusters...${NC}"
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

# Step 6: Wait for clusters to become available (from rollbackRegistering)
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 6: Waiting for clusters to become available...${NC}"
    TIMEOUT=300  # 5 minutes timeout
    INTERVAL=10  # Check every 10 seconds
    ELAPSED=0

    while [ $ELAPSED -lt $TIMEOUT ]; do
        ALL_AVAILABLE=true
        for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
            if [ -n "$CLUSTER" ]; then
                # Check if ManagedClusterConditionAvailable is True
                AVAILABLE=$(kubectl get managedcluster "$CLUSTER" -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status}' 2>/dev/null)
                if [ "$AVAILABLE" != "True" ]; then
                    ALL_AVAILABLE=false
                    echo -e "  ${YELLOW}Waiting for cluster ${CLUSTER} to become available...${NC}"
                fi
            fi
        done

        if [ "$ALL_AVAILABLE" = true ]; then
            echo -e "${GREEN}✓ All clusters are available${NC}"
            break
        fi

        sleep $INTERVAL
        ELAPSED=$((ELAPSED + INTERVAL))
    done

    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo -e "${YELLOW}⚠ Timeout waiting for clusters to become available. Please check cluster status manually.${NC}"
    fi
fi

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}Source Hub Rollback Completed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "\nNext steps:"
echo -e "1. Run the target hub rollback script if needed"
echo -e "2. Verify cluster status: ${YELLOW}kubectl get managedcluster${NC}"
