#!/bin/bash
# Continuously renew managed-cluster-lease to keep mock clusters Available=True.
# Needed for real hub clusters (OCP/ACM) where MCE checks lease renewTime to
# determine cluster availability. Without updates, MCE marks clusters Unknown.
#
# Usage: ./keep-clusters-available.sh <cluster_start:cluster_end> [hub_kubeconfig] [hub_index] [interval_seconds]
# Example: ./keep-clusters-available.sh 1:300 /tmp/myan-hub1 1 60

set -eo pipefail

IFS=':' read -r cluster_start cluster_end <<< "${1:-1:300}"
HUB_KUBECONFIG="${2:-/tmp/myan-hub1}"
hub_index="${3:-1}"
INTERVAL="${4:-60}"
MAX_CONCURRENT=50

# Input validation
if ! [[ "$cluster_start" =~ ^[0-9]+$ ]] || ! [[ "$cluster_end" =~ ^[0-9]+$ ]]; then
    echo "Error: cluster range must be integers (got: ${cluster_start}:${cluster_end})" >&2
    exit 1
fi
if (( cluster_start > cluster_end )); then
    echo "Error: cluster_start (${cluster_start}) must be <= cluster_end (${cluster_end})" >&2
    exit 1
fi
if ! [[ "$INTERVAL" =~ ^[0-9]+$ ]] || (( INTERVAL < 1 )); then
    echo "Error: interval must be a positive integer (got: ${INTERVAL})" >&2
    exit 1
fi

echo ">> Lease keep-alive: hub${hub_index} clusters ${cluster_start}-${cluster_end}, every ${INTERVAL}s"
echo ">> Kubeconfig: ${HUB_KUBECONFIG}"

renew_lease() {
    local cluster_name=$1
    local kubeconfig=$2
    local now
    now=$(date -u +"%Y-%m-%dT%H:%M:%S.000000Z")

    # Update the lease renewTime to simulate klusterlet heartbeat
    kubectl --kubeconfig "$kubeconfig" --request-timeout=10s patch lease managed-cluster-lease \
        -n "$cluster_name" \
        --type=merge \
        --patch "{\"spec\":{\"renewTime\":\"${now}\"}}" 2>/dev/null && echo -n "." || echo -n "x"
}
export -f renew_lease

round=0
while true; do
    round=$((round + 1))
    now=$(date '+%H:%M:%S')
    echo ""
    echo -n ">> Round ${round} (${now}): renewing leases for hub${hub_index} clusters ${cluster_start}-${cluster_end}..."
    jobs=0
    for j in $(seq "$cluster_start" "$cluster_end"); do
        renew_lease "managedcluster-${j}" "$HUB_KUBECONFIG" &
        ((jobs++)) || true
        if ((jobs >= MAX_CONCURRENT)); then
            wait
            jobs=0
        fi
    done
    wait
    echo ""
    echo ">> Round ${round} done. Sleeping ${INTERVAL}s..."
    sleep "$INTERVAL"
done
