set -e

function check() {
  name="$1"
  value="$2"
  if [[ -z "${value}" ]]; then
    echo "Error: environment variable $name must be specified!"
    exit 1
  fi
}

check "KUBECONFIG" "$KUBECONFIG"
check "CONTEXT" "$CONTEXT"

check "IMPORTED1_KUBECONFIG" "$IMPORTED1_KUBECONFIG"
check "IMPORTED1_CONTEXT" "$IMPORTED1_CONTEXT"
check "IMPORTED1_LEAF_HUB_NAME" "$IMPORTED1_LEAF_HUB_NAME"
check "IMPORTED1_NAME" "$IMPORTED1_NAME"

check "IMPORTED2_KUBECONFIG" "$IMPORTED2_KUBECONFIG"
check "IMPORTED2_CONTEXT" "$IMPORTED2_CONTEXT"
check "IMPORTED2_LEAF_HUB_NAME" "$IMPORTED2_LEAF_HUB_NAME"
check "IMPORTED2_NAME" "$IMPORTED2_NAME"

