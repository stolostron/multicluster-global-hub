# !/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
CONFIG_DIR="${ROOT_DIR}/test/resources/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-local.yaml"

HUB_OF_HUB_NAME="global-hub" # the KinD name

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

# hub cluster
hub_kubeconfig="${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$HUB_OF_HUB_NAME" > ${hub_kubeconfig}
hub_api_server=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}" --kubeconfig ${hub_kubeconfig} --context "kind-$HUB_OF_HUB_NAME")
global_hub_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${HUB_OF_HUB_NAME}-control-plane)
hub_namespace="open-cluster-management"

# container nonk8s api server
hub_nonk8s_api_server="http://${global_hub_node_ip}:30080"

# container postgres uri
container_pg_port="32432"
database_uri=$(kubectl get secret multicluster-global-hub-storage -n $hub_namespace --kubeconfig ${hub_kubeconfig} -ojsonpath='{.data.database_uri}' | base64 -d)
container_pg_uri=$(echo $database_uri | sed "s|@.*hoh|@${global_hub_node_ip}:${container_pg_port}/hoh|g")

printf "options:" > $OPTIONS_FILE
printf "\n  hub:" >> $OPTIONS_FILE
printf "\n    name: $HUB_OF_HUB_NAME" >> $OPTIONS_FILE
printf "\n    namespace: ${hub_namespace}" >> $OPTIONS_FILE
printf "\n    apiServer: ${hub_api_server}" >> $OPTIONS_FILE
printf "\n    nonk8sApiServer: ${hub_nonk8s_api_server}" >> $OPTIONS_FILE
printf "\n    kubeconfig: ${hub_kubeconfig}" >> $OPTIONS_FILE
printf "\n    kubecontext: kind-$HUB_OF_HUB_NAME" >> $OPTIONS_FILE
printf '\n    databaseURI: %s' ${container_pg_uri} >> $OPTIONS_FILE # contain $ need to use %s
printf "\n    ManagerImageREF: ${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF}" >> $OPTIONS_FILE
printf "\n    AgentImageREF: ${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF}" >> $OPTIONS_FILE
printf "\n    OperatorImageREF: ${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF}" >> $OPTIONS_FILE
printf "\n  clusters:" >> $OPTIONS_FILE

for i in $(seq 1 "${HUB_CLUSTER_NUM}"); do
  # leafhub
  leafhub_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME}$i"
  kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$LEAF_HUB_NAME$i" > ${leafhub_kubeconfig}
  leafhub_kubecontext=$(kubectl config current-context --kubeconfig ${leafhub_kubeconfig})

  printf "\n    - name: kind-${LEAF_HUB_NAME}$i" >> $OPTIONS_FILE                # if the clusterName = leafhubName, then it is a leafhub
  printf "\n      kubeconfig: ${leafhub_kubeconfig}" >> $OPTIONS_FILE
  printf "\n      leafhubname: kind-${LEAF_HUB_NAME}$i" >> $OPTIONS_FILE
  printf "\n      kubecontext: ${leafhub_kubecontext}" >> $OPTIONS_FILE

  for j in $(seq 1 "${MANAGED_CLUSTER_NUM}"); do
    # imported managedcluster
    managed_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME}$i-cluster$j"
    kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-${LEAF_HUB_NAME}$i-cluster$j" > ${managed_kubeconfig}
    managed_kubecontext=$(kubectl config current-context --kubeconfig ${managed_kubeconfig})

    printf "\n    - name: kind-${LEAF_HUB_NAME}$i-cluster$j" >> $OPTIONS_FILE
    printf "\n      leafhubname: kind-${LEAF_HUB_NAME}$i" >> $OPTIONS_FILE
    printf "\n      kubeconfig: ${managed_kubeconfig}" >> $OPTIONS_FILE
    printf "\n      kubecontext: ${managed_kubecontext}" >> $OPTIONS_FILE
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
