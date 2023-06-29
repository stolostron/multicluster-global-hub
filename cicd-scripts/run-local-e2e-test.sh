# !/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
CONFIG_DIR="${ROOT_DIR}/test/resources/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-local.yaml"

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

# hub cluster
hub_kubeconfig="${CONFIG_DIR}/kubeconfig-${HUB_OF_HUB_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "$HUB_OF_HUB_CTX" > ${hub_kubeconfig}
hub_kubecontext=$(kubectl config current-context --kubeconfig ${hub_kubeconfig})
hub_api_server=$(kubectl config view -o jsonpath="{.clusters[0].cluster.server}" --kubeconfig ${hub_kubeconfig} --context "$HUB_OF_HUB_CTX")
# curl -k -H "Authorization: Bearer ..." https://172.17.0.2:30080/global-hub-api/v1/managedclusters

hub_namespace="open-cluster-management"
container_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${HUB_OF_HUB_NAME})

# container nonk8s api server
hub_nonk8s_api_server="https://${container_node_ip}:30080"

printf "options:" > $OPTIONS_FILE
printf "\n  hub:" >> $OPTIONS_FILE
printf "\n    name: $HUB_OF_HUB_NAME" >> $OPTIONS_FILE
printf "\n    namespace: ${hub_namespace}" >> $OPTIONS_FILE
printf "\n    apiServer: ${hub_api_server}" >> $OPTIONS_FILE
printf "\n    nonk8sApiServer: ${hub_nonk8s_api_server}" >> $OPTIONS_FILE
if [ ! -f "IS_CANARY_ENV" ];then
  printf "\n    kubeconfig: ${ROOT_DIR}/test/setup/config/kubeconfig" >> $OPTIONS_FILE
  printf "\n    crdsDir: ${ROOT_DIR}/pkg/testdata/crds" >> $OPTIONS_FILE
  printf "\n    storagePath: ${ROOT_DIR}/test/setup/hoh/postgres_setup.sh" >> $OPTIONS_FILE
  printf "\n    transportPath: ${ROOT_DIR}/test/setup/hoh/kafka_setup.sh" >> $OPTIONS_FILE
else
  printf "\n    kubeconfig: ${hub_kubeconfig}" >> $OPTIONS_FILE
  printf "\n    storagePath: ${ROOT_DIR}/operator/config/samples/storage/deploy_postgres.sh" >> $OPTIONS_FILE
  printf "\n    transportPath: ${ROOT_DIR}/operator/config/samples/transport/deploy_kafka.sh" >> $OPTIONS_FILE
fi
  printf "\n    kubecontext: ${hub_kubecontext}" >> $OPTIONS_FILE
  printf "\n    databaseExternalHost: ${container_node_ip}" >> $OPTIONS_FILE
  printf "\n    databaseExternalPort: 32432" >> $OPTIONS_FILE
  printf "\n    ManagerImageREF: quay.io/stolostron/multicluster-global-hub-manager:latest" >> $OPTIONS_FILE
  printf "\n    AgentImageREF: quay.io/stolostron/multicluster-global-hub-agent:latest" >> $OPTIONS_FILE
  printf "\n    OperatorImageREF: quay.io/stolostron//multicluster-global-hub-operator:latest" >> $OPTIONS_FILE
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
