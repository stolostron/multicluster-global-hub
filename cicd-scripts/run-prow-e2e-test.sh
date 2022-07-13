#!/usr/bin/env bash

set -euxo pipefail

KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
echo "export KUBECONFIG=$KUBECONFIG" > ${ROOT_DIR}/test/resources/env.list
echo "export LOG=$LOG" >> ${ROOT_DIR}/test/resources/env.list

ssh "${OPT[@]}" "$HOST" sudo yum install gcc git -y
scp "${OPT[@]}" -r ../hub-of-hubs "$HOST:/tmp/hub-of-hubs"
ssh "${OPT[@]}" "$HOST" "cd /tmp/hub-of-hubs && srouce /tmp/hub-of-hubs/test/resources/env.list && make e2e-setup-start && make e2e-tests-all"
