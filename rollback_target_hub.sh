#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Target Hub Rollback Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Step 1: Get migration name
read -p "Enter the Migration CR name: " MIGRATION_NAME
if [ -z "$MIGRATION_NAME" ]; then
    echo -e "${RED}Error: Migration name cannot be empty${NC}"
    exit 1
fi

# Step 2: Get target hub name from migration CR
echo -e "\n${YELLOW}Fetching target hub name from migration CR...${NC}"
TARGET_HUB_NAME=$(kubectl get managedclustermigration "$MIGRATION_NAME" -o jsonpath='{.spec.targetHub}' 2>/dev/null)

if [ -z "$TARGET_HUB_NAME" ]; then
    echo -e "${RED}Error: Could not retrieve target hub name from migration CR${NC}"
    echo -e "${YELLOW}Please make sure the migration CR exists and has .spec.targetHub set${NC}"
    exit 1
fi

echo -e "${GREEN}Target Hub Name: ${TARGET_HUB_NAME}${NC}"
read -p "Is this correct? (yes/no): " CONFIRM
if [ "$CONFIRM" != "yes" ]; then
    echo -e "${YELLOW}Please enter the correct target hub name.${NC}"
    read -p "Target Hub Name: " TARGET_HUB_NAME
    if [ -z "$TARGET_HUB_NAME" ]; then
        echo -e "${RED}Error: Target hub name cannot be empty${NC}"
        exit 1
    fi
    echo -e "${GREEN}Using Target Hub Name: ${TARGET_HUB_NAME}${NC}"
fi

# Step 3: Get Global Hub namespace
GH_NAMESPACE=${GH_NAMESPACE:-"multicluster-global-hub"}
echo -e "\n${YELLOW}Using Global Hub namespace: ${GH_NAMESPACE}${NC}"

# Step 4: Get list of clusters to delete from target hub
echo -e "\n${YELLOW}Getting list of clusters from migration ConfigMap...${NC}"
FAILED_CLUSTERS=$(kubectl get configmap "$MIGRATION_NAME" -n "$GH_NAMESPACE" -o jsonpath='{.data.failure}' 2>/dev/null || echo "")

if [ -z "$FAILED_CLUSTERS" ]; then
    echo -e "${YELLOW}No failed clusters found in ConfigMap.${NC}"
    echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
    read -p "Enter cluster names (comma-separated) or press Enter to show all clusters from target hub: " MANUAL_CLUSTERS

    if [ -z "$MANUAL_CLUSTERS" ]; then
        # Show all managedclusters on target hub for user to review
        echo -e "\n${YELLOW}Current ManagedClusters on target hub:${NC}"
        kubectl get managedcluster -o custom-columns=NAME:.metadata.name,HUB-ACCEPTED:.spec.hubAcceptsClient,JOINED:.status.conditions[?(@.type==\"ManagedClusterJoined\")].status
        echo ""
        echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
        read -p "Enter cluster names to delete (comma-separated): " MANUAL_CLUSTERS
    fi

    CLUSTERS_TO_DELETE="$MANUAL_CLUSTERS"
else
    echo -e "${GREEN}Failed clusters from ConfigMap: ${FAILED_CLUSTERS}${NC}"
    echo ""
    read -p "Use this cluster list? (yes/no): " USE_CONFIGMAP_LIST

    if [ "$USE_CONFIGMAP_LIST" = "yes" ]; then
        CLUSTERS_TO_DELETE="$FAILED_CLUSTERS"
    else
        echo -e "${YELLOW}Please enter the cluster names to delete from target hub.${NC}"
        echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
        read -p "Enter cluster names (comma-separated) or press Enter to show all clusters: " MANUAL_CLUSTERS

        if [ -z "$MANUAL_CLUSTERS" ]; then
            # Show all managedclusters on target hub for user to review
            echo -e "\n${YELLOW}Current ManagedClusters on target hub:${NC}"
            kubectl get managedcluster -o custom-columns=NAME:.metadata.name,HUB-ACCEPTED:.spec.hubAcceptsClient,JOINED:.status.conditions[?(@.type==\"ManagedClusterJoined\")].status
            echo ""
            echo -e "${YELLOW}Example format: cluster1,cluster2,cluster3${NC}"
            read -p "Enter cluster names to delete (comma-separated): " MANUAL_CLUSTERS
        fi

        CLUSTERS_TO_DELETE="$MANUAL_CLUSTERS"
    fi
fi

if [ -z "$CLUSTERS_TO_DELETE" ]; then
    echo -e "${YELLOW}No clusters specified for deletion${NC}"
    CLUSTER_ARRAY=()
else
    # Convert comma-separated list to array
    IFS=',' read -ra CLUSTER_ARRAY <<< "$CLUSTERS_TO_DELETE"
fi

# Step 5: Delete ManagedServiceAccount from GlobalHub (same cluster)
echo -e "\n${YELLOW}Step 1: Deleting ManagedServiceAccount from GlobalHub...${NC}"
MSA_NAME="$MIGRATION_NAME"
kubectl delete managedserviceaccount "$MSA_NAME" -n "$TARGET_HUB_NAME" 2>/dev/null && \
    echo -e "${GREEN}✓ ManagedServiceAccount deleted: ${MSA_NAME}${NC}" || \
    echo -e "${YELLOW}⚠ ManagedServiceAccount not found or already deleted: ${MSA_NAME}${NC}"

# Step 6: Delete ManagedClusters (with user confirmation)
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step 2: Deleting ManagedClusters from target hub...${NC}"
    echo -e "${RED}WARNING: The following ManagedClusters will be DELETED:${NC}"
    printf '  %s\n' "${CLUSTER_ARRAY[@]}"
    echo ""
    read -p "Are you absolutely sure you want to delete these clusters? (yes/no): " DELETE_CONFIRM

    if [ "$DELETE_CONFIRM" != "yes" ]; then
        echo -e "${RED}Cluster deletion cancelled by user${NC}"
        echo -e "${YELLOW}Skipping ManagedCluster and KlusterletAddonConfig deletion...${NC}"
    else
        for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
            if [ -n "$CLUSTER" ]; then
                echo -e "  Deleting ManagedCluster: ${CLUSTER}"
                kubectl delete managedcluster "$CLUSTER" 2>/dev/null && \
                    echo -e "    ${GREEN}✓ Deleted${NC}" || \
                    echo -e "    ${YELLOW}⚠ Not found or already deleted${NC}"
            fi
        done
        echo -e "${GREEN}✓ ManagedClusters deletion completed${NC}"

        # Step 7: Delete KlusterletAddonConfigs
        echo -e "\n${YELLOW}Step 3: Deleting KlusterletAddonConfigs...${NC}"
        for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
            if [ -n "$CLUSTER" ]; then
                echo -e "  Deleting KlusterletAddonConfig: ${CLUSTER}"
                kubectl delete klusterletaddonconfig "$CLUSTER" -n "$CLUSTER" 2>/dev/null && \
                    echo -e "    ${GREEN}✓ Deleted${NC}" || \
                    echo -e "    ${YELLOW}⚠ Not found or already deleted${NC}"
            fi
        done
        echo -e "${GREEN}✓ KlusterletAddonConfigs deletion completed${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No clusters specified, skipping ManagedCluster deletion${NC}"
fi

# Step 8: Remove auto-approve user from ClusterManager
echo -e "\n${YELLOW}Step 4: Removing auto-approve user from ClusterManager...${NC}"
MSA_USER="system:serviceaccount:${TARGET_HUB_NAME}:${MIGRATION_NAME}"
echo -e "  MSA User to remove: ${MSA_USER}"

# Get current autoApproveUsers list
CURRENT_USERS=$(kubectl get clustermanager cluster-manager -o jsonpath='{.spec.registrationConfiguration.autoApproveUsers}' 2>/dev/null || echo "")

if echo "$CURRENT_USERS" | grep -q "$MSA_USER"; then
    echo -e "${YELLOW}  Found MSA user in autoApproveUsers list. Please remove it manually:${NC}"
    echo -e "  ${YELLOW}kubectl edit clustermanager cluster-manager${NC}"
    echo -e "  ${YELLOW}Remove the line: ${MSA_USER}${NC}"
    read -p "Press Enter after you have removed the user (or skip if already removed)..."
    echo -e "${GREEN}✓ Auto-approve user removal acknowledged${NC}"
else
    echo -e "${GREEN}✓ MSA user not found in autoApproveUsers (already removed or never added)${NC}"
fi

# Step 9: Delete RBAC resources
echo -e "\n${YELLOW}Step 5: Deleting migration RBAC resources...${NC}"

# Delete ClusterRole for SAR
CLUSTERROLE_SAR="global-hub-migration-${MIGRATION_NAME}-sar"
kubectl delete clusterrole "$CLUSTERROLE_SAR" 2>/dev/null && \
    echo -e "${GREEN}✓ ClusterRole deleted: ${CLUSTERROLE_SAR}${NC}" || \
    echo -e "${YELLOW}⚠ ClusterRole not found: ${CLUSTERROLE_SAR}${NC}"

# Delete ClusterRoleBinding for SAR
CLUSTERROLEBINDING_SAR="global-hub-migration-${MIGRATION_NAME}-sar"
kubectl delete clusterrolebinding "$CLUSTERROLEBINDING_SAR" 2>/dev/null && \
    echo -e "${GREEN}✓ ClusterRoleBinding deleted: ${CLUSTERROLEBINDING_SAR}${NC}" || \
    echo -e "${YELLOW}⚠ ClusterRoleBinding not found: ${CLUSTERROLEBINDING_SAR}${NC}"

# Delete ClusterRoleBinding for Registration
CLUSTERROLEBINDING_REG="global-hub-migration-${MIGRATION_NAME}-registration"
kubectl delete clusterrolebinding "$CLUSTERROLEBINDING_REG" 2>/dev/null && \
    echo -e "${GREEN}✓ ClusterRoleBinding deleted: ${CLUSTERROLEBINDING_REG}${NC}" || \
    echo -e "${YELLOW}⚠ ClusterRoleBinding not found: ${CLUSTERROLEBINDING_REG}${NC}"

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}Target Hub Rollback Completed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "\nNext steps:"
echo -e "1. Verify that RBAC resources have been cleaned up"
echo -e "2. Check if ManagedServiceAccount was deleted: ${YELLOW}kubectl get managedserviceaccount -n ${TARGET_HUB_NAME}${NC}"
echo -e "3. Verify ManagedClusters on target hub: ${YELLOW}kubectl get managedcluster${NC}"
