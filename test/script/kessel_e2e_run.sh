#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)

# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

kind_cluster_name="global-hub-kessel"
kind_cluster_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${kind_cluster_name}-control-plane)
inventory_namespace=${INVENTORY_NAMESPACE:-"multicluster-global-hub"}

# create a nodeport to expose the inventory api
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: inventory-api-external
  namespace: $inventory_namespace
spec:
  type: NodePort
  ports:
    - name: http-server
      port: 8081
      protocol: TCP
      targetPort: 8081
      nodePort: 30081  # NodePort for HTTP server
    - name: grpc-server
      port: 9081
      protocol: TCP
      targetPort: 9081
      nodePort: 30082  # NodePort for gRPC server
  selector:
    name: inventory-api
EOF

http_url="${kind_cluster_ip}:30081"
kubectl get secret inventory-api-server-ca-certs -n "$inventory_namespace" -ojsonpath='{.data.ca\.crt}' | base64 -d >/tmp/ca.crt
kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.crt}' | base64 -d >/tmp/client.crt
kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.key}' | base64 -d >/tmp/client.key

cat <<EOF >"$CURRENT_DIR/rest.yaml"
host: $http_url
ca.crt: $(kubectl get secret inventory-api-server-ca-certs -n "$inventory_namespace" -ojsonpath='{.data.ca\.crt}')
client.crt: $(kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.crt}')
client.key: $(kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.key}')
EOF

transport_config_name=transport-config-guest
kubectl delete secret $transport_config_name -n "$inventory_namespace" --ignore-not-found
kubectl create secret generic $transport_config_name -n "$inventory_namespace" \
  --from-file=rest.yaml="$CURRENT_DIR/rest.yaml"
rm "$CURRENT_DIR/rest.yaml"
echo "inventory rest api configuration is ready!"

# Go programs typically use dynamic linking for C libraries: confluent-kafka package is used in e2e test
export CGO_ENABLED=1
export KUBECONFIG=${CONFIG_DIR}/${kind_cluster_name}

# hub cluster
OPTION_FILE="${CONFIG_DIR}/kessel-options.yaml"
cat <<EOF >"$OPTION_FILE"
options:
  namespace: $inventory_namespace 
  transportconfig: $transport_config_name
  kubeconfig: "$CONFIG_DIR/global-hub-kessel"
  kafkauser: global-hub-kafka-user
  kafkatopic: kessel-inventory
  kafkacluster: kafka
EOF

ginkgo -v --fail-fast "$TEST_DIR/e2e/kessel" --output-dir="$CONFIG_DIR" --junit-report=report.xml -- -options="$OPTION_FILE"
