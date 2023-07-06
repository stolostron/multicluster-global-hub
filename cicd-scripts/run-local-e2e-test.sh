# !/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
CONFIG_DIR="${ROOT_DIR}/test/resources/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-local.yaml"

export TAG=${TAG:-"latest"}
export OPENSHIFT_CI=${OPENSHIFT_CI:-"false"}
export REGISTRY=${REGISTRY:-"quay.io/stolostron"}

if [[ $OPENSHIFT_CI == "false" ]]; then
  export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-manager:${TAG}"}
  export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-agent:$TAG"}
  export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF=${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF:-"${REGISTRY}/multicluster-global-hub-operator:$TAG"}
fi

HUB_OF_HUB_NAME="hub-of-hubs" # the container name
HUB_OF_HUB_CTX="microshift"

# KinD cluster, the context with prefix 'kind-'
LEAF_HUB_NAME="hub"
# MANAGED_NAME="hub1-cluster1"
HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-2}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-1}

if [ ! -d "$CONFIG_DIR" ];then
  mkdir -p "$CONFIG_DIR"
fi

if [ ! -f "$KUBECONFIG" ];then
  KUBECONFIG=${ROOT_DIR}/test/setup/config/kubeconfig
  echo "using default KUBECONFIG = $KUBECONFIG"
fi

hub_namespace="open-cluster-management"

printf "options:" > $OPTIONS_FILE
printf "\n  hub:" >> $OPTIONS_FILE
printf "\n    kubeconfig: ${ROOT_DIR}/test/setup/config/kubeconfig" >> $OPTIONS_FILE
printf "\n  clusters:" >> $OPTIONS_FILE

for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  # leafhub
  leafhub_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME}$i"
  kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$LEAF_HUB_NAME$i" > ${leafhub_kubeconfig}
  leafhub_kubecontext=$(kubectl config current-context --kubeconfig ${leafhub_kubeconfig})
  printf "\n   - kubeconfig: ${leafhub_kubeconfig}" >> $OPTIONS_FILE
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

if [ -z "${filter}" ]; then
  ginkgo --label-filter="!e2e-tests-prune" --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
else
  ginkgo --label-filter=${filter} --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
fi

cat ${ROOT_DIR}/test/resources/report/report.xml | grep failures=\"0\" | grep errors=\"0\" > /dev/null
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ${ROOT_DIR}/test/resources/report/report.xml
    exit 1
fi
