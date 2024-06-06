#!/bin/bash

set -euox pipefail

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
# shellcheck source=/dev/null
source "$CURRENT_DIR/common.sh"

export CURRENT_DIR
export GH_NAME="global-hub"
export GH_CTX="kind-global-hub"
export MH_NUM=${MH_NUM:-2}
export MC_NUM=${MC_NUM:-1}

# setup kubeconfig
export KUBE_DIR=${CURRENT_DIR}/kubeconfig
check_dir "$KUBE_DIR"
export KUBECONFIG=${KUBECONFIG:-${KUBE_DIR}/kind-clusters}

# Init clusters
echo -e "$BLUE creating clusters $NC"
start_time=$(date +%s)
kind_cluster "$GH_NAME" 2>&1 &
for i in $(seq 1 "${MH_NUM}"); do
  hub_name="hub$i"
  kind_cluster "$hub_name" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    cluster_name="hub$i-cluster$j"
    kind_cluster "$cluster_name" 2>&1 &
  done
done
wait
end_time=$(date +%s)
sum_time=$((end_time - start_time))
echo -e "$YELLOW creating clusters:$NC $sum_time seconds"

# Init hub resources
start_time=$(date +%s)

# GH 
export GH_KUBECONFIG=$KUBE_DIR/kind-$GH_NAME
start_time=$(date +%s)

# expose the server so that the spoken cluster can use the kubeconfig to connect it:  governance-policy-framework-addon
node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${GH_NAME}-control-plane)
kubectl --kubeconfig "$GH_KUBECONFIG" config set-cluster kind-$GH_NAME --server="https://$node_ip:6443"
for i in $(seq 1 "$MH_NUM"); do 
  node_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "hub$i-control-plane")
  kubectl --kubeconfig "$KUBE_DIR/kind-hub$i" config set-cluster "kind-hub$i" --server="https://$node_ip:6443"
done

# init resources
echo -e "$BLUE initilize resources $NC"
install_crds $GH_CTX 2>&1 &    # router, mch(not needed for the managed clusters)
enable_service_ca $GH_CTX $GH_NAME "$CURRENT_DIR/hoh" 2>&1 &

for i in $(seq 1 "${MH_NUM}"); do
  install_crds "kind-hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    install_crds "kind-hub$i-cluster$j" 2>&1 &
  done
done

# Install posgres 
bash "$CURRENT_DIR"/hoh/postgres/postgres_setup.sh "$GH_KUBECONFIG" 2>&1 &
echo "$!" > "$KUBE_DIR/PID"

# init hubs
echo -e "$BLUE initializing hubs $NC"
init_hub $GH_CTX &
for i in $(seq 1 "${MH_NUM}"); do
  init_hub "kind-hub$i" &
done

wait
end_time=$(date +%s)
echo -e "$YELLOW Initializing hubs:$NC $((end_time - start_time)) seconds"

# Importing clusters
start_time=$(date +%s)

# join spoken clusters, if the cluster has been joined, then skip
echo -e "$BLUE importing clusters $NC"

for i in $(seq 1 "${MH_NUM}"); do
  join_cluster $GH_CTX "kind-hub$i" 2>&1 &  # join to global hub
  for j in $(seq 1 "${MC_NUM}"); do
    join_cluster "kind-hub$i" "kind-hub$i-cluster$j" 2>&1 & # join to managed hub
  done
done

wait
end_time=$(date +%s)
echo -e "$YELLOW Importing cluster:$NC $((end_time - start_time)) seconds"

# Install kafka and other resources
bash "$CURRENT_DIR"/hoh/kafka/kafka_setup.sh "$GH_KUBECONFIG" 2>&1 &
echo "$!" >> "$KUBE_DIR/PID"

# Install app and policy
start_time=$(date +%s)

# app
echo -e "$BLUE deploying app $NC"

# deploy the subscription operators to the hub cluster
clusteradm install hub-addon --names application-manager --context "$GH_CTX" 2>&1 
for i in $(seq 1 "${MH_NUM}"); do
  init_app $GH_CTX "kind-hub$i" 2>&1
  # enable the addon on the managed clusters
  clusteradm addon enable --names application-manager --clusters "kind-hub$i" --context "$GH_CTX"

  clusteradm install hub-addon --names application-manager --context "kind-hub$i" 2>&1
  for j in $(seq 1 "${MC_NUM}"); do
    init_app "kind-hub$i" "kind-hub$i-cluster$j" 2>&1 
    clusteradm addon enable --names application-manager --clusters "kind-hub$i-cluster$j" --context "kind-hub$i" 2>&1 
  done
done

# policy 
echo -e "$BLUE deploying policy $NC"
for i in $(seq 1 "${MH_NUM}"); do
  init_policy $GH_CTX "kind-hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    init_policy "kind-hub$i" "kind-hub$i-cluster$j" 2>&1 &
  done
done

# wait all the backend process ready
wait 

end_time=$(date +%s)
echo -e "$YELLOW App and policy:$NC $((end_time - start_time)) seconds"

echo -e "$BLUE enable the clusters $NC"
for i in $(seq 1 "${MH_NUM}"); do
  enable_cluster $GH_CTX "kind-hub$i" 2>&1 &
  for j in $(seq 1 "${MC_NUM}"); do
    enable_cluster "kind-hub$i" "kind-hub$i-cluster$j" 2>&1 &
  done
done

wait

#need the following labels to enable deploying agent in leaf hub cluster
for i in $(seq 1 "${MH_NUM}"); do
  echo -e "$GREEN [Access the Clusters]: export KUBECONFIG=$KUBE_DIR/kind-hub$i $NC"
  for j in $(seq 1 "${MC_NUM}"); do
    echo -e "$GREEN [Access the Clusters]: export KUBECONFIG=$KUBE_DIR/kind-hub$i-cluster$j $NC"
  done
done
echo -e "$GREEN [Access the Clusters]: export KUBECONFIG=$KUBECONFIG $NC"
