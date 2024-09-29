#!/bin/bash

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)

# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

KUBECONFIG=${1:-$KUBECONFIG}        # the kubeconfig for running the inventory server
SECRET_KUBECONFIG=${2:-$KUBECONFIG} # the kubeconfig for generating the inventory connection secret

inventory_namespace=${INVENTORY_NAMESPACE:-"multicluster-global-hub"}
secret_namespace=${SECRET_NAMESPACE:-"open-cluster-management"}
secret_name=${SECRET_NAME:-"transport-config"}

# kubectl get secret inventory-api-server-ca-certs -n "$inventory_namespace" -ojsonpath='{.data.ca\.crt}' | base64 -d > /tmp/ca.crt
# kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.crt}' | base64 -d > /tmp/client.crt
# kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.key}' | base64 -d > /tmp/client.key

host=$(kubectl get route inventory-api -n "$inventory_namespace" -ojsonpath='{.spec.host}')
cat <<EOF >"$CURRENT_DIR/rest.yaml"
host: https://${host}:443
ca.crt: $(kubectl get secret inventory-api-server-ca-certs -n "$inventory_namespace" -ojsonpath='{.data.ca\.crt}')
client.crt: $(kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.crt}')
client.key: $(kubectl get secret inventory-api-guest-certs -n "$inventory_namespace" -ojsonpath='{.data.tls\.key}')
EOF

kubectl create secret generic "$secret_name" -n "$secret_namespace" --kubeconfig "$SECRET_KUBECONFIG" \
  --from-file=rest.yaml="$CURRENT_DIR/rest.yaml"
rm "$CURRENT_DIR/rest.yaml"
echo "restful configuration is ready!"
