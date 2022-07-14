#!/usr/bin/env bash

set -euxo pipefail

KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
HOST_DIR="/tmp/hub-of-hubs"
echo "export LOG_MODE=DEBUG" >> ${ROOT_DIR}/test/resources/env.list
echo "ROOT_DIR=${ROOT_DIR}"

ssh "${OPT[@]}" "$HOST" sudo yum install gcc git wget  -y 

# update the go version to 1.17+
ssh "${OPT[@]}" "$HOST" sudo rm -rf /usr/local/go     
ssh "${OPT[@]}" "$HOST" sudo wget https://dl.google.com/go/go1.17.7.linux-amd64.tar.gz 
ssh "${OPT[@]}" "$HOST" sudo tar -C /usr/local/ -xvf go1.17.7.linux-amd64.tar.gz 
echo "go version: $(go version)"

ssh "${OPT[@]}" "$HOST" echo "PATH=$PATH"
ssh "${OPT[@]}" "$HOST" sudo wget https://github.com/open-cluster-management-io/clusteradm/releases/download/v0.2.0/clusteradm_linux_amd64.tar.gz
ssh "${OPT[@]}" "$HOST" sudo tar -C /usr/local/bin -xvf clusteradm_linux_amd64.tar.gz
ssh "${OPT[@]}" "$HOST" export "PATH=${PATH}:/usr/local/bin"
ssh "${OPT[@]}" "$HOST" echo "PATH=$PATH"

echo "clusteradm help: $(clusteradm help)" 

scp "${OPT[@]}" -r ../hub-of-hubs "$HOST:$HOST_DIR"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && make e2e-setup-start && make e2e-tests-all"

