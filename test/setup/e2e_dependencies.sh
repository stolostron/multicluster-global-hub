#!/usr/bin/env bash

binDir="/usr/bin"

CURRENT_DIR=$(cd "$(dirname "$0")" || exit;pwd)
source ${CURRENT_DIR}/common.sh

startTime_s=`date +%s`
check_golang
check_docker
if [[ $OPENSHIFT_CI == "true" ]]; then 
  check_volume
fi
check_kind
check_kubectl
check_clusteradm
check_ginkgo
endTime_s=`date +%s`
sumTime=$[ $endTime_s - $startTime_s ]
echo "dependencies :$sumTime seconds"
