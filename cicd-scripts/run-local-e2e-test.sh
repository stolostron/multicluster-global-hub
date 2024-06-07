#!/bin/bash

set -ex

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
KUBE_DIR="${ROOT_DIR}/test/setup/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-local.yaml"

GH_NAME="global-hub" # the KinD name
MH_NAME="hub"
MH_NUM=${MH_NUM:-2}
MC_NUM=${MC_NUM:-1}

if [ ! -d "$KUBE_DIR" ];then
  mkdir -p "$KUBE_DIR"
fi

export KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/kind-clusters}

# hub cluster
GH_KUBECONFIG="${KUBE_DIR}/kind-${GH_NAME}"
kubectl config view --raw --minify --kubeconfig "$KUBECONFIG" --context "kind-$GH_NAME" > "$GH_KUBECONFIG"
hub_api_server=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}" --kubeconfig "$GH_KUBECONFIG" --context "kind-$GH_NAME")
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${GH_NAME}-control-plane)
GH_NAMESPACE="multicluster-global-hub"

# container nonk8s api server
hub_nonk8s_api_server="http://${global_hub_node_ip}:30080"

# container postgres uri
container_pg_port="32432"
database_uri=$(kubectl get secret multicluster-global-hub-storage -n "$GH_NAMESPACE" --kubeconfig "$GH_KUBECONFIG" -ojsonpath='{.data.database_uri}' | base64 -d)
container_pg_uri=$(echo "$database_uri" | sed "s|@.*hoh|@${global_hub_node_ip}:${container_pg_port}/hoh|g")

cat <<EOF > "$OPTIONS_FILE"
options:
  globalhub:
    name: $GH_NAME
    namespace: ${GH_NAMESPACE}
    apiServer: ${hub_api_server}
    nonk8sApiServer: ${hub_nonk8s_api_server}
    kubeconfig: ${GH_KUBECONFIG}
    kubecontext: kind-$GH_NAME
    databaseURI: ${container_pg_uri}
    managerImageREF: ${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF}
    agentImageREF: ${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF}
    operatorImageREF: ${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF}
    managedhubs:
EOF

for i in $(seq 1 "${MH_NUM}"); do
  # leafhub
  mh_kubeconfig="${KUBE_DIR}/kind-${MH_NAME}$i"
  kubectl config view --raw --minify --kubeconfig "${KUBECONFIG}" --context "kind-$MH_NAME$i" > "${mh_kubeconfig}"
  mh_kubecontext=$(kubectl config current-context --kubeconfig "${mh_kubeconfig}")

cat <<EOF >> "$OPTIONS_FILE"
    - name: kind-${MH_NAME}$i
      kubeconfig: ${mh_kubeconfig}
      kubecontext: ${mh_kubecontext}
EOF

for j in $(seq 1 "${MC_NUM}"); do
  # imported managedcluster
  mc_kubeconfig="${KUBE_DIR}/${MH_NAME}$i-cluster$j"
  kubectl config view --raw --minify --kubeconfig "${KUBECONFIG}" --context "kind-${MH_NAME}$i-cluster$j" > "${mc_kubeconfig}"
  mc_kubecontext=$(kubectl config current-context --kubeconfig "${mc_kubeconfig}")

  cat <<EOF >> "$OPTIONS_FILE"
      managedclusters:
      - name: kind-${MH_NAME}$i-cluster$j
        leafhubname: kind-${MH_NAME}$i
        kubeconfig: ${mc_kubeconfig}
        kubecontext: ${mc_kubecontext}
EOF
  done
done

while getopts ":f:v:" opt; do
  case $opt in
    f) filter="$OPTARG"
    ;;
    v) verbose="$OPTARG"
    ;;
    \?) echo "Invalid option -$OPTARG" >&2
    exit 1
    ;;
  esac

  case $OPTARG in
    -*) echo "Option $opt needs a valid argument"
    exit 1
    ;;
  esac
done

verbose=${verbose:=5}

report_dir="$ROOT_DIR/test/resources/report"
if [ -z "${filter}" ]; then
  ginkgo --fail-fast --label-filter="!e2e-test-prune" --output-dir="$report_dir" --json-report=report.json \
  --junit-report=report.xml "$ROOT_DIR/test/pkg/e2e" -- -options="$OPTIONS_FILE" -v="$verbose"
else
  ginkgo --fail-fast --label-filter="${filter}" --output-dir="$report_dir" --json-report=report.json \
  --junit-report=report.xml "$ROOT_DIR"/test/pkg/e2e -- -options="$OPTIONS_FILE" -v="$verbose"
fi


if ! cat "$report_dir/report.xml" | grep failures=\"0\" | grep errors=\"0\" > /dev/null
then
    echo "Cannot pass all test cases."
    cat "$report_dir/report.xml"
    exit 1
fi
