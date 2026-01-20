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

# Step 1: Get migration name from available ManagedClusterMigration CRs
echo -e "${YELLOW}Fetching available ManagedClusterMigration CRs...${NC}"
MIGRATIONS=$(kubectl get managedclustermigration -A --no-headers 2>/dev/null)

if [ -z "$MIGRATIONS" ]; then
    echo -e "${RED}Error: No ManagedClusterMigration CRs found in the cluster${NC}"
    echo -e "${YELLOW}Please create a ManagedClusterMigration CR first or check your kubeconfig${NC}"
    exit 1
fi

# Display available migrations with numbers
echo -e "\n${GREEN}Available ManagedClusterMigration CRs:${NC}"
echo "----------------------------------------"
i=1
declare -a MIGRATION_NAMES
while IFS= read -r line; do
    NAMESPACE=$(echo "$line" | awk '{print $1}')
    NAME=$(echo "$line" | awk '{print $2}')
    MIGRATION_NAMES+=("$NAME")
    echo -e "  ${GREEN}[$i]${NC} $NAME (namespace: $NAMESPACE)"
    ((i++))
done <<< "$MIGRATIONS"
echo "----------------------------------------"

# Let user select a migration
read -p "Select a migration (1-$((i-1))) or enter name manually: " SELECTION

if [[ "$SELECTION" =~ ^[0-9]+$ ]] && [ "$SELECTION" -ge 1 ] && [ "$SELECTION" -le $((i-1)) ]; then
    MIGRATION_NAME="${MIGRATION_NAMES[$((SELECTION-1))]}"
    echo -e "${GREEN}Selected: ${MIGRATION_NAME}${NC}"
else
    # User entered a name manually
    MIGRATION_NAME="$SELECTION"
fi

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

# ============================================================================
# Rollback Execution
# This matches the rollbackDeploying() function in migration_to_syncer.go
# ============================================================================

STEP_NUM=1

# Step 1: Remove all migration resources for each cluster
# This matches: removeMigrationResources() in migration_to_syncer.go
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step ${STEP_NUM}: Removing migration resources for clusters...${NC}"
    echo -e "${RED}WARNING: The following clusters will have their migration resources DELETED:${NC}"
    printf '  %s\n' "${CLUSTER_ARRAY[@]}"
    echo ""
    read -p "Are you absolutely sure you want to delete these resources? (yes/no): " DELETE_CONFIRM

    if [ "$DELETE_CONFIRM" != "yes" ]; then
        echo -e "${RED}Migration resources deletion cancelled by user${NC}"
    else
        for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
            if [ -n "$CLUSTER" ]; then
                echo -e "\n${YELLOW}  Processing cluster: ${CLUSTER}${NC}"

                # Delete ManagedCluster (cluster.open-cluster-management.io/v1)
                echo -e "    Deleting ManagedCluster: ${CLUSTER}"
                kubectl delete managedcluster "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted ManagedCluster${NC}" || \
                    echo -e "      ${YELLOW}⚠ ManagedCluster not found or error${NC}"

                # Delete KlusterletAddonConfig (agent.open-cluster-management.io/v1)
                echo -e "    Deleting KlusterletAddonConfig: ${CLUSTER}"
                kubectl delete klusterletaddonconfig "$CLUSTER" -n "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted KlusterletAddonConfig${NC}" || \
                    echo -e "      ${YELLOW}⚠ KlusterletAddonConfig not found or error${NC}"

                # Delete migration secrets
                echo -e "    Deleting migration secrets in namespace ${CLUSTER}..."
                for SECRET_SUFFIX in "-admin-password" "-admin-kubeconfig" "-metadata-json" "-seed-reconfiguration"; do
                    SECRET_NAME="${CLUSTER}${SECRET_SUFFIX}"
                    kubectl delete secret "$SECRET_NAME" -n "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                        echo -e "      ${GREEN}✓ Deleted secret: ${SECRET_NAME}${NC}" || true
                done

                # Delete secrets with siteconfig sync-wave annotation
                echo -e "    Deleting siteconfig sync-wave secrets..."
                kubectl get secrets -n "$CLUSTER" -o json 2>/dev/null | \
                    jq -r '.items[] | select(.metadata.annotations["siteconfig.open-cluster-management.io/sync-wave"] != null) | .metadata.name' 2>/dev/null | \
                    while read -r SECRET_NAME; do
                        if [ -n "$SECRET_NAME" ]; then
                            kubectl delete secret "$SECRET_NAME" -n "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                                echo -e "      ${GREEN}✓ Deleted sync-wave secret: ${SECRET_NAME}${NC}" || true
                        fi
                    done

                # Delete secrets with siteconfig preserve label
                echo -e "    Deleting siteconfig preserve secrets..."
                kubectl delete secrets -n "$CLUSTER" -l "siteconfig.open-cluster-management.io/preserve" --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted preserve-labeled secrets${NC}" || true

                # Delete configmaps with siteconfig preserve label
                echo -e "    Deleting siteconfig preserve configmaps..."
                kubectl delete configmaps -n "$CLUSTER" -l "siteconfig.open-cluster-management.io/preserve" --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted preserve-labeled configmaps${NC}" || true

                # Delete BareMetalHost resources (metal3.io/v1alpha1)
                echo -e "    Deleting BareMetalHost resources..."
                kubectl delete baremetalhost -n "$CLUSTER" --all --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted BareMetalHost resources${NC}" || true

                # Delete HostFirmwareSettings resources (metal3.io/v1alpha1)
                echo -e "    Deleting HostFirmwareSettings resources..."
                kubectl delete hostfirmwaresettings -n "$CLUSTER" --all --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted HostFirmwareSettings resources${NC}" || true

                # Delete FirmwareSchema resources (metal3.io/v1alpha1)
                echo -e "    Deleting FirmwareSchema resources..."
                kubectl delete firmwareschema -n "$CLUSTER" --all --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted FirmwareSchema resources${NC}" || true

                # Delete HostFirmwareComponents resources (metal3.io/v1alpha1)
                echo -e "    Deleting HostFirmwareComponents resources..."
                kubectl delete hostfirmwarecomponents -n "$CLUSTER" --all --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted HostFirmwareComponents resources${NC}" || true

                # Delete ImageClusterInstall resources (extensions.hive.openshift.io/v1alpha1)
                echo -e "    Deleting ImageClusterInstall: ${CLUSTER}..."
                kubectl delete imageclusterinstall "$CLUSTER" -n "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                    echo -e "      ${GREEN}✓ Deleted ImageClusterInstall${NC}" || true

                # Delete ClusterDeployment resources (hive.openshift.io/v1)
                # Add pause annotation and remove deprovision finalizers before deletion
                echo -e "    Deleting ClusterDeployment: ${CLUSTER}..."
                if kubectl get clusterdeployment "$CLUSTER" -n "$CLUSTER" &>/dev/null; then
                    # Add pause annotation to prevent reprovisioning
                    echo -e "      Adding pause annotation to ClusterDeployment..."
                    kubectl annotate clusterdeployment "$CLUSTER" -n "$CLUSTER" \
                        "siteconfig.open-cluster-management.io/deploy-action-pause=true" --overwrite 2>/dev/null || true

                    # Remove deprovision finalizers to prevent blocking deletion
                    echo -e "      Removing deprovision finalizers..."
                    kubectl patch clusterdeployment "$CLUSTER" -n "$CLUSTER" --type=json \
                        -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true

                    # Delete the ClusterDeployment
                    kubectl delete clusterdeployment "$CLUSTER" -n "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                        echo -e "      ${GREEN}✓ Deleted ClusterDeployment${NC}" || \
                        echo -e "      ${YELLOW}⚠ ClusterDeployment deletion may be pending${NC}"
                else
                    echo -e "      ${YELLOW}⚠ ClusterDeployment not found${NC}"
                fi

                echo -e "    ${GREEN}✓ Completed migration resources removal for cluster: ${CLUSTER}${NC}"
            fi
        done
        echo -e "\n${GREEN}✓ Migration resources removal completed${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No clusters specified, skipping migration resources removal${NC}"
fi
((STEP_NUM++))

# Step 2: Remove cluster namespaces
# This matches the namespace deletion in rollbackDeploying()
if [ ${#CLUSTER_ARRAY[@]} -gt 0 ]; then
    echo -e "\n${YELLOW}Step ${STEP_NUM}: Removing cluster namespaces...${NC}"
    echo -e "${RED}WARNING: This will delete the following namespaces:${NC}"
    printf '  %s\n' "${CLUSTER_ARRAY[@]}"
    echo ""
    read -p "Delete these namespaces? (yes/no): " DELETE_NS_CONFIRM

    if [ "$DELETE_NS_CONFIRM" = "yes" ]; then
        for CLUSTER in "${CLUSTER_ARRAY[@]}"; do
            if [ -n "$CLUSTER" ]; then
                echo -e "  Deleting namespace: ${CLUSTER}"
                kubectl delete namespace "$CLUSTER" --ignore-not-found=true 2>/dev/null && \
                    echo -e "    ${GREEN}✓ Deleted namespace${NC}" || \
                    echo -e "    ${YELLOW}⚠ Namespace not found or error${NC}"
            fi
        done
        echo -e "${GREEN}✓ Namespace removal completed${NC}"
    else
        echo -e "${YELLOW}⚠ Namespace deletion skipped by user${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No clusters specified, skipping namespace removal${NC}"
fi
((STEP_NUM++))

# Step 3: Rollback initializing stage
# This matches: rollbackInitializing() in migration_to_syncer.go
echo -e "\n${YELLOW}Step ${STEP_NUM}: Rolling back initializing stage...${NC}"

# 3a. Remove MSA user from ClusterManager AutoApproveUsers list
# This matches: removeAutoApproveUser() in migration_to_syncer.go
echo -e "\n  ${YELLOW}Removing auto-approve user from ClusterManager...${NC}"
MSA_USER="system:serviceaccount:${TARGET_HUB_NAME}:${MIGRATION_NAME}"
echo -e "    MSA User to remove: ${MSA_USER}"

# Get current autoApproveUsers list
CURRENT_USERS=$(kubectl get clustermanager cluster-manager -o jsonpath='{.spec.registrationConfiguration.autoApproveUsers}' 2>/dev/null || echo "")

if echo "$CURRENT_USERS" | grep -q "$MSA_USER"; then
    echo -e "  ${YELLOW}Found MSA user in autoApproveUsers list. Attempting to remove...${NC}"

    # Use JSON patch to remove the specific user from the array
    # First, get the current list and find the index
    USER_INDEX=$(kubectl get clustermanager cluster-manager -o json 2>/dev/null | \
        jq -r --arg user "$MSA_USER" '.spec.registrationConfiguration.autoApproveUsers | to_entries | .[] | select(.value == $user) | .key' 2>/dev/null)

    if [ -n "$USER_INDEX" ]; then
        kubectl patch clustermanager cluster-manager --type=json \
            -p="[{\"op\":\"remove\",\"path\":\"/spec/registrationConfiguration/autoApproveUsers/${USER_INDEX}\"}]" 2>/dev/null && \
            echo -e "    ${GREEN}✓ Removed MSA user from ClusterManager autoApproveUsers${NC}" || \
            echo -e "    ${RED}✗ Failed to patch ClusterManager. Please remove manually:${NC}
    kubectl edit clustermanager cluster-manager
    Remove the line: ${MSA_USER}"
    else
        echo -e "    ${YELLOW}⚠ Could not determine user index. Please remove manually:${NC}"
        echo -e "    kubectl edit clustermanager cluster-manager"
        echo -e "    Remove the line: ${MSA_USER}"
        read -p "Press Enter after you have removed the user (or skip if already removed)..."
    fi
else
    echo -e "    ${GREEN}✓ MSA user not found in autoApproveUsers (already removed or never added)${NC}"
fi

# 3b. Clean up RBAC resources
# This matches: cleanupMigrationRBAC() in migration_to_syncer.go
echo -e "\n  ${YELLOW}Cleaning up migration RBAC resources...${NC}"

# Delete ClusterRole for SubjectAccessReview: global-hub-migration-<msa-name>-sar
CLUSTERROLE_SAR="global-hub-migration-${MIGRATION_NAME}-sar"
kubectl delete clusterrole "$CLUSTERROLE_SAR" --ignore-not-found=true 2>/dev/null && \
    echo -e "    ${GREEN}✓ Deleted ClusterRole: ${CLUSTERROLE_SAR}${NC}" || \
    echo -e "    ${YELLOW}⚠ ClusterRole not found: ${CLUSTERROLE_SAR}${NC}"

# Delete ClusterRoleBinding for SubjectAccessReview: global-hub-migration-<msa-name>-sar
CLUSTERROLEBINDING_SAR="global-hub-migration-${MIGRATION_NAME}-sar"
kubectl delete clusterrolebinding "$CLUSTERROLEBINDING_SAR" --ignore-not-found=true 2>/dev/null && \
    echo -e "    ${GREEN}✓ Deleted ClusterRoleBinding: ${CLUSTERROLEBINDING_SAR}${NC}" || \
    echo -e "    ${YELLOW}⚠ ClusterRoleBinding not found: ${CLUSTERROLEBINDING_SAR}${NC}"

# Delete ClusterRoleBinding for Agent Registration: global-hub-migration-<msa-name>-registration
CLUSTERROLEBINDING_REG="global-hub-migration-${MIGRATION_NAME}-registration"
kubectl delete clusterrolebinding "$CLUSTERROLEBINDING_REG" --ignore-not-found=true 2>/dev/null && \
    echo -e "    ${GREEN}✓ Deleted ClusterRoleBinding: ${CLUSTERROLEBINDING_REG}${NC}" || \
    echo -e "    ${YELLOW}⚠ ClusterRoleBinding not found: ${CLUSTERROLEBINDING_REG}${NC}"

echo -e "\n${GREEN}✓ Initializing stage rollback completed${NC}"
((STEP_NUM++))

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}Target Hub Rollback Completed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "\nRollback Summary:"
echo -e "  - Removed migration resources (secrets, configmaps, ZTP resources)"
echo -e "  - Deleted ManagedClusters and KlusterletAddonConfigs"
echo -e "  - Removed cluster namespaces"
echo -e "  - Cleaned up ClusterManager autoApproveUsers"
echo -e "  - Deleted migration RBAC resources"
echo -e "\nVerification commands:"
echo -e "  ${YELLOW}kubectl get managedcluster${NC}"
echo -e "  ${YELLOW}kubectl get clusterrolebinding | grep global-hub-migration${NC}"
echo -e "  ${YELLOW}kubectl get clustermanager cluster-manager -o jsonpath='{.spec.registrationConfiguration.autoApproveUsers}'${NC}"
