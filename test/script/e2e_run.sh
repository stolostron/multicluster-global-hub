#!/bin/bash

set -euo pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
TEST_DIR=$(dirname "$CURRENT_DIR")

CONFIG_DIR="${CURRENT_DIR}/config"
OPTION_FILE="${CONFIG_DIR}/options.yaml"

[ -d "$CONFIG_DIR" ] || (mkdir -p "$CONFIG_DIR")

export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}
export KUBECONFIG=${KUBECONFIG:-${CONFIG_DIR}/clusters}
export GH_NAME="global-hub" # the KinD name
export GH_KUBECONFIG="${CONFIG_DIR}/${GH_NAME}"
export GH_NAMESPACE="multicluster-global-hub"

# hub cluster
hub_api_server=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}" --kubeconfig "$GH_KUBECONFIG" --context "$GH_NAME")
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${GH_NAME}-control-plane)

# container nonk8s api server
hub_nonk8s_api_server="http://${global_hub_node_ip}:30080"

cat <<EOF >"$OPTION_FILE"
options:
  globalhub:
    name: $GH_NAME
    namespace: ${GH_NAMESPACE}
    apiServer: ${hub_api_server}
    nonk8sApiServer: ${hub_nonk8s_api_server}
    kubeconfig: ${GH_KUBECONFIG}
    kubecontext: $GH_NAME
    managerImageREF: ${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF}
    agentImageREF: ${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF}
    operatorImageREF: ${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF}
    managedhubs:
EOF

for i in $(seq 1 "${MH_NUM}"); do
  # leafhub
  mh_kubeconfig="${CONFIG_DIR}/hub$i"
  mh_kubecontext=$(kubectl config current-context --kubeconfig "${mh_kubeconfig}")

  cat <<EOF >>"$OPTION_FILE"
    - name: hub$i
      kubeconfig: $mh_kubeconfig
      kubecontext: $mh_kubecontext
EOF

  for j in $(seq 1 "${MC_NUM}"); do
    # imported managedcluster
    mc_kubeconfig="${CONFIG_DIR}/hub$i-cluster$j"
    mc_kubecontext=$(kubectl config current-context --kubeconfig "${mc_kubeconfig}")

    cat <<EOF >>"$OPTION_FILE"
      managedclusters:
      - name: hub$i-cluster$j
        leafhubname: hub$i
        kubeconfig: $mc_kubeconfig
        kubecontext: $mc_kubecontext
EOF
  done
done

while getopts ":f:v:" opt; do
  case $opt in
  f)
    filter="$OPTARG"
    ;;
  v)
    verbose="$OPTARG"
    ;;
  \?)
    echo "Invalid option -$OPTARG" >&2
    exit 1
    ;;
  esac

  case $OPTARG in
  -*)
    echo "Option $opt needs a valid argument"
    exit 1
    ;;
  esac
done

verbose=${verbose:=5}

if [ -z "${filter}" ]; then
  ginkgo --fail-fast --label-filter="!e2e-test-prune" --output-dir="$CONFIG_DIR" --json-report=report.json \
    --junit-report=report.xml "$TEST_DIR/e2e" -- -options="$OPTION_FILE" -v="$verbose"
else
  ginkgo --fail-fast --label-filter="${filter}" --output-dir="$CONFIG_DIR" --json-report=report.json \
    --junit-report=report.xml "$TEST_DIR"/e2e -- -options="$OPTION_FILE" -v="$verbose"
fi

if ! cat "$CONFIG_DIR/report.xml" | grep failures=\"0\" | grep errors=\"0\" >/dev/null; then
  echo "Cannot pass all test cases."
  cat "$CONFIG_DIR/report.xml"
  exit 1
fi
