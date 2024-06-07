#!/bin/bash

set -euox pipefail

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

export CURRENT_DIR
export GH_NAME="global-hub"
export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}

# setup kubeconfig
export KUBE_DIR=${CURRENT_DIR}/kubeconfig
check_dir "$KUBE_DIR"
export KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/clusters}

# Init clusters
echo -e "$BLUE create clusters $NC"
start_time=$(date +%s)

kind_cluster "$GH_NAME" 2>&1 &
for i in $(seq 1 "${MH_NUM}"); do
  kind_cluster "hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    kind_cluster "hub$i-cluster$j" 2>&1 &
  done
done

wait

end_time=$(date +%s)
echo -e "$YELLOW creating clusters:$NC $((end_time - start_time)) seconds"

# Init hub resources
start_time=$(date +%s)

# GH
echo -e "$BLUE initialize global hub setting $NC"
export GH_KUBECONFIG=$KUBE_DIR/$GH_NAME
# expose the server so that the spoken cluster can use the kubeconfig to connect it:  governance-policy-framework-addon
node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${GH_NAME}-control-plane)
kubectl --kubeconfig "$GH_KUBECONFIG" config set-cluster $GH_NAME --server="https://$node_ip:6443"
enable_service_ca $GH_NAME "$CURRENT_DIR/resource" 2>&1 &
install_crds $GH_NAME 2>&1 & # router, mch(not needed for the managed clusters)



# for i in $(seq 1 "$MH_NUM"); do
#   node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "hub$i-control-plane")
#   kubectl --kubeconfig "$KUBE_DIR/hub$i" config set-cluster "hub$i" --server="https://$node_ip:6443"
# done

start_time=$(date +%s)

bash "$CURRENT_DIR/resource/postgres/postgres_setup.sh" "$GH_KUBECONFIG" 2>&1 &
echo "$!" >"$KUBE_DIR/PID"
bash "$CURRENT_DIR"/resource/kafka/kafka_setup.sh "$GH_KUBECONFIG" 2>&1 &
echo "$!" >>"$KUBE_DIR/PID"

# start_time=$(date +%s)
# # init hubs
# echo -e "$BLUE initializing hubs $NC"
# init_hub $GH_NAME &
# for i in $(seq 1 "${MH_NUM}"); do
#   init_hub "hub$i" &
# done
# wait
# end_time=$(date +%s)
# echo -e "$YELLOW Initializing hubs:$NC $((end_time - start_time)) seconds"

for i in $(seq 1 "${MH_NUM}"); do
  install_crds "hub$i" 2>&1 

  bash "$CURRENT_DIR"/ocm_setup.sh "$GH_NAME" "hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    install_crds "hub$i-cluster$j" 2>&1 

    bash "$CURRENT_DIR"/ocm_setup.sh "hub$i" "hub$i-cluster$j" 2>&1 &
  done
done


wait
end_time=$(date +%s)
echo -e "$YELLOW init ocm, app and policy:$NC $((end_time - start_time)) seconds"

# # Install app and policy
# # app
# echo -e "$BLUE deploying app $NC"

# # deploy the subscription operators to the hub cluster
# for i in $(seq 1 "${MH_NUM}"); do
#   init_app $GH_NAME "hub$i" 2>&1
#   for j in $(seq 1 "${MC_NUM}"); do
#     init_app "hub$i" "hub$i-cluster$j" 2>&1
#   done
# done

# # policy
# echo -e "$BLUE deploying policy $NC"
# for i in $(seq 1 "${MH_NUM}"); do
#   init_policy $GH_NAME "hub$i" 2>&1
#   for j in $(seq 1 "${MC_NUM}"); do
#     init_policy "hub$i" "hub$i-cluster$j" 2>&1
#   done
# done
# end_time=$(date +%s)
# echo -e "$YELLOW App and policy:$NC $((end_time - start_time)) seconds"

set +x

start_time=$(date +%s)

echo -e "$BLUE validate ocm, app and policy $NC"
for i in $(seq 1 "${MH_NUM}"); do
  wait_ocm $GH_NAME "hub$i"
  enable_cluster $GH_NAME "hub$i" 2>&1

  wait_policy $GH_NAME "hub$i"
  wait_application $GH_NAME "hub$i"
  for j in $(seq 1 "${MC_NUM}"); do
    wait_ocm "hub$i" "hub$i-cluster$j"
    enable_cluster "hub$i" "hub$i-cluster$j" 2>&1

    wait_policy "hub$i" "hub$i-cluster$j"
    wait_application "hub$i" "hub$i-cluster$j"
  done
done

end_time=$(date +%s)
echo -e "$YELLOW validate ocm, app and policy:$NC $((end_time - start_time)) seconds"


for i in $(seq 1 "${MH_NUM}"); do
  echo -e "$CYAN [Access the ManagedHub]: export KUBECONFIG=$KUBE_DIR/hub$i $NC"
  for j in $(seq 1 "${MC_NUM}"); do
    echo -e "$CYAN [Access the ManagedCluster]: export KUBECONFIG=$KUBE_DIR/hub$i-cluster$j $NC"
  done
done
echo -e "$BOLD_GREEN [Access the Clusters]: export KUBECONFIG=$KUBECONFIG $NC"
