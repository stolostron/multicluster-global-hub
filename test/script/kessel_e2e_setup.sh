#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

cluster_name="global-hub-kessel"

kind_cluster $cluster_name
install_crds $cluster_name
enable_service_ca $cluster_name "$TEST_DIR/manifest"
enable_olm $cluster_name

# Build images
cd "$PROJECT_DIR" || exit
MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-operator:latest"
MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-manager:latest"
MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF="image-registry.testing/stolostron/multicluster-global-hub-agent:latest"
docker build . -t $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF -f operator/Dockerfile
docker build . -t $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF -f manager/Dockerfile
docker build . -t $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF -f agent/Dockerfile

# Load to kind cluster
kind load docker-image $MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF --name $cluster_name
kind load docker-image $MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF --name $cluster_name
#kind load docker-image $MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF --name $cluster_name

# Replace to use the built images
sed -i -e "s;quay.io/stolostron/multicluster-global-hub-manager:latest;$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF;" ./operator/config/manager/manager.yaml
sed -i -e "s;quay.io/stolostron/multicluster-global-hub-agent:latest;$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF;" ./operator/config/manager/manager.yaml
# Update imagepullpolicy to IfNotPresent
sed -i -e "s;imagePullPolicy: Always;imagePullPolicy: IfNotPresent;" ./operator/config/manager/manager.yaml

# deploy serviceMonitor CRD
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${cluster_name}-control-plane)

# Deploy global hub operator and operand
cd operator || exit
make deploy IMG=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF
cat <<EOF | kubectl apply -f -
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    global-hub.open-cluster-management.io/catalog-source-name: operatorhubio-catalog
    global-hub.open-cluster-management.io/catalog-source-namespace: olm
    global-hub.open-cluster-management.io/with-inventory: ""
    global-hub.open-cluster-management.io/kind-cluster-ip: "$global_hub_node_ip"
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
spec:
  availabilityConfig: Basic
  dataLayer:
    kafka:
      topics:
        specTopic: gh-spec
        statusTopic: gh-event.*
      storageSize: 1Gi
    postgres:
      retention: 18m
      storageSize: 1Gi
  enableMetrics: false
  imagePullPolicy: IfNotPresent
EOF

# Create a temporary file to store stderr messages
error_cache=$(mktemp)

# Wait the control planes are ready
# Use a Subshell to Isolate the Exit
(wait_cmd "kubectl get deploy/multicluster-global-hub-operator -n multicluster-global-hub --context $cluster_name") 2>>"$error_cache" || true
(wait_cmd "kubectl get deploy/multicluster-global-hub-manager -n multicluster-global-hub --context $cluster_name") 2>>"$error_cache" || true
(kubectl wait deploy/multicluster-global-hub-manager -n multicluster-global-hub --for condition=Available=True --timeout=60s --context "$cluster_name") || true
(wait_cmd "kubectl get deploy/inventory-api -n multicluster-global-hub --context $cluster_name" 60) 2>>"$error_cache" || true
(kubectl wait deploy/inventory-api -n multicluster-global-hub --for condition=Available=True --timeout=60s --context $cluster_name) || true

# Debug information
kubectl get kafka -n multicluster-global-hub -oyaml --context $cluster_name || true
kubectl get pod -n multicluster-global-hub --context $cluster_name || true
kubectl get mcgh -n multicluster-global-hub -oyaml --context $cluster_name || true
kubectl logs deploy/multicluster-global-hub-operator -n multicluster-global-hub --context $cluster_name || true
kubectl get deploy -n multicluster-global-hub --context $cluster_name || true

if [ -s "$error_cache" ]; then
    echo "The following errors were encountered:"
    cat "$error_cache"
    rm "$error_cache"  # Clean up the temporary file
    exit 1             # Exit with error status
else
    echo "All commands executed successfully."
    rm "$error_cache"  # Clean up the temporary file
fi

