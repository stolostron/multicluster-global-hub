# !/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
CONFIG_DIR="${ROOT_DIR}/test/resources/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-ocp.yaml"

HUB_OF_HUB_NAME="hub-of-hubs" # the container name
#HUB_OF_HUB_CTX=""

# KinD cluster, the context with prefix 'kind-'
LEAF_HUB_NAME1="hub1"
LEAF_HUB_NAME2="hub2"
MANAGED1_NAME="hub1-cluster1"
MANAGED2_NAME="hub2-cluster1"
HUB_CLUSTER_NUM=${HUB_CLUSTER_NUM:-2}
MANAGED_CLUSTER_NUM=${MANAGED_CLUSTER_NUM:-1}

if [ ! -d "$CONFIG_DIR" ];then
  mkdir -p "$CONFIG_DIR"
fi

# hub cluster
hub_kubeconfig="${ROOTDIR}/resources/kubeconfig/kubeconfig-ocp-$HUB_OF_HUB_NAME"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context ${HUB_OF_HUB_CTX} > ${hub_kubeconfig}
hub_kubecontext=$(kubectl config current-context --kubeconfig ${hub_kubeconfig})
hub_api_server=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}" --kubeconfig ${hub_kubeconfig} --context "$HUB_OF_HUB_CTX")

hub_app_domain=$(kubectl -n openshift-ingress-operator get ingresscontrollers default -ojsonpath='{.status.domain}'  --kubeconfig ${KUBECONFIG} --context ${CONTEXT})
hub_base_domain="${hub_app_domain#apps.}"
hub_nonk8s_api_server="https://multicluster-global-hub-manager.apps.${hub_base_domain}"

hub_namespace="multicluster-global-hub"
hub_database_secret="hub-of-hubs-database-secret"

# imported managedcluster1
managed1_kubeconfig="${CONFIG_DIR}/kubeconfig-ocp-${MANAGED1_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$MANAGED1_NAME" > ${managed1_kubeconfig}
managed1_kubecontext=$(kubectl config current-context --kubeconfig ${managed1_kubeconfig})

# imported managedcluster2
managed2_kubeconfig="${CONFIG_DIR}/kubeconfig-ocp-${MANAGED2_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$MANAGED2_NAME" > ${managed2_kubeconfig}
managed2_kubecontext=$(kubectl config current-context --kubeconfig ${managed2_kubeconfig})

# leafhub1
leafhub1_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME1}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$LEAF_HUB_NAME1" > ${leafhub1_kubeconfig}
leafhub1_kubecontext=$(kubectl config current-context --kubeconfig ${leafhub1_kubeconfig})

# leafhub2
leafhub2_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME2}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$LEAF_HUB_NAME2" > ${leafhub2_kubeconfig}
leafhub2_kubecontext=$(kubectl config current-context --kubeconfig ${leafhub2_kubeconfig})

printf "options:" > $OPTIONS_FILE
printf "\n  globalhub:" >> $OPTIONS_FILE
printf "\n    name: $HUB_OF_HUB_NAME" >> $OPTIONS_FILE
printf "\n    namespace: ${hub_namespace}" >> $OPTIONS_FILE
printf "\n    apiServer: ${hub_api_server}" >> $OPTIONS_FILE
printf "\n    nonk8sApiServer: ${hub_nonk8s_api_server}" >> $OPTIONS_FILE
printf "\n    kubeconfig: ${hub_kubeconfig}" >> $OPTIONS_FILE
printf "\n    kubecontext: ${hub_kubecontext}" >> $OPTIONS_FILE
printf "\n    databaseSecret: ${hub_database_secret}" >> $OPTIONS_FILE
printf "\n    managedhubs:" >> $OPTIONS_FILE
printf "\n    - name: kind-${MANAGED1_NAME}" >> $OPTIONS_FILE
printf "\n      kubeconfig: ${managed1_kubeconfig}" >> $OPTIONS_FILE
printf "\n      kubecontext: ${managed1_kubecontext}" >> $OPTIONS_FILE
printf "\n      managedclusters:" >> $OPTIONS_FILE
printf "\n      - name: kind-${MANAGED2_NAME}" >> $OPTIONS_FILE
printf "\n        leafhubname: kind-${LEAF_HUB_NAME2}" >> $OPTIONS_FILE
printf "\n        kubeconfig: ${managed2_kubeconfig}" >> $OPTIONS_FILE
printf "\n        kubecontext: ${managed2_kubecontext}" >> $OPTIONS_FILE
printf "\n    - name: kind-${LEAF_HUB_NAME1}" >> $OPTIONS_FILE                # if the clusterName = leafhubName, then it is a leafhub
printf "\n      kubeconfig: ${leafhub1_kubeconfig}" >> $OPTIONS_FILE
printf "\n      kubecontext: ${leafhub1_kubecontext}" >> $OPTIONS_FILE
printf "\n      managedclusters:" >> $OPTIONS_FILE
printf "\n      - name: kind-${LEAF_HUB_NAME2}" >> $OPTIONS_FILE                # if the clusterName = leafhubName, then it is a leafhub
printf "\n        kubeconfig: ${leafhub2_kubeconfig}" >> $OPTIONS_FILE
printf "\n        leafhubname: kind-${LEAF_HUB_NAME2}" >> $OPTIONS_FILE
printf "\n        kubecontext: ${leafhub2_kubecontext}" >> $OPTIONS_FILE

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

if [ -z "$filter" ]; then
  ginkgo --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
else
  ginkgo --label-filter="$filter" --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
fi