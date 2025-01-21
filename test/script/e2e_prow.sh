#!/usr/bin/env bash

set -euo pipefail

KEY="${SHARED_DIR}/private.pem"
chmod 400 "${KEY}"

IP="$(cat "${SHARED_DIR}/public_ip")"
HOST="ec2-user@${IP}"
OPT=(-q -o "UserKnownHostsFile=/dev/null" -o "StrictHostKeyChecking=no" -i "${KEY}")
PROJECT_DIR="$(
  cd "$(dirname "$0")/../.."
  pwd -P
)"
HOST_DIR="/tmp/multicluster-global-hub"

echo "export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF=$MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF" >>${PROJECT_DIR}/test/script/env.list
echo "export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF=$MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF" >>${PROJECT_DIR}/test/script/env.list
echo "export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF=$MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF" >>${PROJECT_DIR}/test/script/env.list
echo "export OPENSHIFT_CI=$OPENSHIFT_CI" >>${PROJECT_DIR}/test/script/env.list
echo "export VERBOSE=6" >>${PROJECT_DIR}/test/script/env.list

scp "${OPT[@]}" -r ../multicluster-global-hub "$HOST:$HOST_DIR"

ssh "${OPT[@]}" "$HOST" sudo yum install git wget jq librdkafka gcc -y
# Insufficient resources creating kind clusters, modify parameters to expand
ssh "${OPT[@]}" "$HOST" "sudo sh -c 'echo \"fs.inotify.max_user_watches=524288\" >> /etc/sysctl.conf && \
                                     echo \"fs.inotify.max_user_instances=8192\" >> /etc/sysctl.conf && \
                                     sysctl -p /etc/sysctl.conf'"

echo "setup e2e environment"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && sudo make e2e-dep && make e2e-setup" > >(tee "$ARTIFACT_DIR/e2e-setup.log") 2>&1

trap 'echo "An error occurred" >&2; log_component_logs; exit 1;' ERR
log_component_logs() {
  ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && make e2e-log/operator" > >(tee "$ARTIFACT_DIR/e2e-operator.log")
  ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && make e2e-log/manager" > >(tee "$ARTIFACT_DIR/e2e-manager.log")
  ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && make e2e-log/grafana" > >(tee "$ARTIFACT_DIR/e2e-grafana.log")
  ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && make e2e-log/agent" > >(tee "$ARTIFACT_DIR/e2e-agent.log")
}

echo "runn e2e tests"
ssh "${OPT[@]}" "$HOST" "cd $HOST_DIR && . test/script/env.list && make e2e-test-all" > >(tee "$ARTIFACT_DIR/e2e-test.log") 2>&1

# If the script reaches this point, it means no error occurred
trap - ERR
