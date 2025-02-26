#!/bin/bash
# Copyright (c) 2023 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -eo pipefail

# creating the simulated managedcluster
for i in $(seq 1 $1)
do
    echo "Deleting Simulated managedCluster managedcluster-${i}..."
    kubectl delete managedcluster managedcluster-${i} || true
done