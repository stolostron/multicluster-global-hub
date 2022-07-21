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
echo "export HUB_OF_HUBS_MANAGER_IMAGE_REF=$HUB_OF_HUBS_MANAGER_IMAGE_REF" >> ${ROOT_DIR}/test/resources/env.list
echo "export HUB_OF_HUBS_AGENT_IMAGE_REF=$HUB_OF_HUBS_AGENT_IMAGE_REF" >> ${ROOT_DIR}/test/resources/env.list
echo "export OPENSHIFT_CI=$OPENSHIFT_CI" >> ${ROOT_DIR}/test/resources/env.list

echo "install dependencies"
ssh "${OPT[@]}" "$HOST" sudo yum install gcc git wget  -y 
scp "${OPT[@]}" -r ../hub-of-hubs "$HOST:$HOST_DIR"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && sudo make e2e-setup-dependencies"
echo "setup e2e environment"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && make e2e-setup-start"
sleep 1800
echo "runn e2e tests"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && make e2e-tests-all"