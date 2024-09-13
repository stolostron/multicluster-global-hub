#!/bin/bash

CURRENT_DIR=$(
  cd "$(dirname "$0")" || exit
  pwd
)
# shellcheck source=/dev/null
source "$CURRENT_DIR/util.sh"

start_time=$(date +%s)
check_golang
check_docker
if [[ $OPENSHIFT_CI == "true" ]]; then
  check_volume
fi
check_kind
check_kubectl
check_clusteradm
check_ginkgo
check_helm
end_time=$(date +%s)
sum_time=$((end_time - start_time))
echo -e "$YELLOW dependencies:$NC $sum_time seconds"
