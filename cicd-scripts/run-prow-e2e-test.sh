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
ssh "${OPT[@]}" "$HOST" sudo rm /usr/local/go -y          # remove old go with version 1.16
scp "${OPT[@]}" -r ../hub-of-hubs "$HOST:$HOST_DIR"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && make e2e-setup-start && make e2e-tests-all"

