# !/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
CONFIG_DIR="${ROOT_DIR}/test/resources/kubeconfig"
OPTIONS_FILE="${ROOT_DIR}/test/resources/options-local.yaml"

HUB_OF_HUB_NAME="hub-of-hubs" # the container name
HUB_OF_HUB_CTX="microshift"

# KinD cluster, the context with prefix 'kind-'
LEAF_HUB_NAME="hub1"
MANAGED1_NAME="hub1-cluster1"
MANAGED2_NAME="hub1-cluster2"

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
# curl -k -H "Authorization: Bearer ..." https://172.17.0.2:30080/multicloud/hub-of-hubs-nonk8s-api/managedclusters

container_node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${HUB_OF_HUB_NAME})
hub_nonk8s_api_server="https://${container_node_ip}:30080" 
hub_namespace="open-cluster-management"
hub_database_secret="hub-of-hubs-database-secret"

# imported managedcluster1
managed1_kubeconfig="${CONFIG_DIR}/kubeconfig-${MANAGED1_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$MANAGED1_NAME" > ${managed1_kubeconfig}
managed1_kubecontext=$(kubectl config current-context --kubeconfig ${managed1_kubeconfig})

# imported managedcluster2
managed2_kubeconfig="${CONFIG_DIR}/kubeconfig-${MANAGED2_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$MANAGED2_NAME" > ${managed2_kubeconfig}
managed2_kubecontext=$(kubectl config current-context --kubeconfig ${managed2_kubeconfig})

# leafhub 
leafhub_kubeconfig="${CONFIG_DIR}/kubeconfig-${LEAF_HUB_NAME}"
kubectl config view --raw --minify --kubeconfig ${KUBECONFIG} --context "kind-$LEAF_HUB_NAME" > ${leafhub_kubeconfig}
leafhub_kubecontext=$(kubectl config current-context --kubeconfig ${leafhub_kubeconfig})

printf "options:" > $OPTIONS_FILE
printf "\n  hub:" >> $OPTIONS_FILE
printf "\n    name: $HUB_OF_HUB_NAME" >> $OPTIONS_FILE
printf "\n    namespace: ${hub_namespace}" >> $OPTIONS_FILE
printf "\n    apiServer: ${hub_api_server}" >> $OPTIONS_FILE
printf "\n    nonk8sApiServer: ${hub_nonk8s_api_server}" >> $OPTIONS_FILE
printf "\n    kubeconfig: ${hub_kubeconfig}" >> $OPTIONS_FILE
printf "\n    kubecontext: ${hub_kubecontext}" >> $OPTIONS_FILE
printf "\n    databaseSecret: ${hub_database_secret}" >> $OPTIONS_FILE
printf "\n  clusters:" >> $OPTIONS_FILE
printf "\n    - name: kind-${MANAGED1_NAME}" >> $OPTIONS_FILE
printf "\n      leafhubname: kind-${LEAF_HUB_NAME}" >> $OPTIONS_FILE
printf "\n      kubeconfig: ${managed1_kubeconfig}" >> $OPTIONS_FILE
printf "\n      kubecontext: ${managed1_kubecontext}" >> $OPTIONS_FILE
printf "\n    - name: kind-${MANAGED2_NAME}" >> $OPTIONS_FILE
printf "\n      leafhubname: kind-${LEAF_HUB_NAME}" >> $OPTIONS_FILE
printf "\n      kubeconfig: ${managed2_kubeconfig}" >> $OPTIONS_FILE
printf "\n      kubecontext: ${managed2_kubecontext}" >> $OPTIONS_FILE
printf "\n    - name: kind-${LEAF_HUB_NAME}" >> $OPTIONS_FILE                # if the clusterName = leafhubName, then it is a leafhub
printf "\n      kubeconfig: ${leafhub_kubeconfig}" >> $OPTIONS_FILE
printf "\n      leafhubname: kind-${LEAF_HUB_NAME}" >> $OPTIONS_FILE
printf "\n      kubecontext: ${leafhub_kubecontext}" >> $OPTIONS_FILE

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

if command -v go &> /dev/null; then
  echo "install go version 1.7 or higher"
  wget https://dl.google.com/go/go1.17.7.linux-amd64.tar.gz
  tar -C /usr/local/ -xvf go1.17.7.linux-amd64.tar.gz
  export GOROOT=/usr/local/go 
fi
echo "go version: $(go version)"

if command -v ginkgo &> /dev/null; then
    GINKGO_CMD=ginkgo
else
    # just for prow vm
    go install github.com/onsi/ginkgo/v2/ginkgo@latest
    go get github.com/onsi/gomega/...
    GINKGO_CMD="$(go env GOPATH)/bin/ginkgo"
fi
echo "ginkgo version: $($GINKGO_CMD version)"

if [ -z "${filter}" ]; then
  ${GINKGO_CMD} --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
else
  ${GINKGO_CMD} --label-filter=${filter} --output-dir="${ROOT_DIR}/test/resources/report" --json-report=report.json \
  --junit-report=report.xml ${ROOT_DIR}/test/pkg/e2e -- -options=$OPTIONS_FILE -v="$verbose"
fi

cat ${ROOT_DIR}/test/resources/report/report.xml | grep failures=\"0\" | grep errors=\"0\" > /dev/null
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ${ROOT_DIR}/test/resources/report/report.xml
    exit 1
fi
