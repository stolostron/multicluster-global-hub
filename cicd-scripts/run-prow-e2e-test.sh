#!/usr/bin/env bash

set -euxo pipefail

KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")

ROOT_DIR="$(cd "$(dirname "$0")/.." ; pwd -P)"
HOST_DIR="/tmp/hub-of-hubs"

echo "export MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF=$MULTICLUSTER_GLOBALHUB_MANAGER_IMAGE_REF" >> ${ROOT_DIR}/test/resources/env.list
echo "export MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF=$MULTICLUSTER_GLOBALHUB_AGENT_IMAGE_REF" >> ${ROOT_DIR}/test/resources/env.list
echo "export OPENSHIFT_CI=$OPENSHIFT_CI" >> ${ROOT_DIR}/test/resources/env.list
echo "export LOG=/dev/stdout" >> ${ROOT_DIR}/test/resources/env.list
echo "export LEAF_HUB_LOG=/dev/stdout" >> ${ROOT_DIR}/test/resources/env.list
echo "export VERBOSE=6" >> ${ROOT_DIR}/test/resources/env.list

scp "${OPT[@]}" -r ../hub-of-hubs "$HOST:$HOST_DIR"

ssh "${OPT[@]}" "$HOST" sudo yum install gcc git wget  -y 
echo "setup e2e environment"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && sudo make e2e-setup-dependencies && make e2e-setup-start" > >(tee "$ARTIFACT_DIR/run-e2e-setup.log") 2>&1
echo "runn e2e tests"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/resources/env.list && make e2e-tests-all" > >(tee "$ARTIFACT_DIR/run-e2e-test.log") 2>&1
