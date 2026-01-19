# E2E Environment Code Update Guide

## Overview

This document describes how to update code and deploy changes to the e2e test environment.

## Prerequisites

- Local development machine with the hub-of-hubs repository
- SSH access to `cloud-vm` (e2e build server)
- E2E environment already set up
- E2e test update in local should also sync in to the e2e build server, where is also the e2e test is run on, the code should be the latest version.

## Workflow

### Step 1: Make Code Changes Locally

```bash
# Switch to e2e branch
git checkout fix-migration-ocm-e2e

# Make your code changes
# ...

# Commit changes
git add <files>
git commit -s -m "your commit message"

# Push to GitHub
git push origin fix-migration-ocm-e2e
```

### Step 2: Pull Code on Build Server

```bash
ssh cloud-vm "cd ~/workspace/hub-of-hubs && git stash && git pull origin fix-migration-ocm-e2e"
```

### Step 3: Build and Push Images

```bash
# Build and push agent image
ssh cloud-vm "bash -c 'source ~/.bash_profile 2>/dev/null; export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin; cd ~/workspace/hub-of-hubs && export REGISTRY=quay.io/myan && make build-agent-image push-agent-image'"

# Build and push manager image (if needed)
ssh cloud-vm "bash -c 'source ~/.bash_profile 2>/dev/null; export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin; cd ~/workspace/hub-of-hubs && export REGISTRY=quay.io/myan && make build-manager-image push-manager-image'"

# Build and push operator image (if needed)
ssh cloud-vm "bash -c 'source ~/.bash_profile 2>/dev/null; export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin; cd ~/workspace/hub-of-hubs && export REGISTRY=quay.io/myan && make build-operator-image push-operator-image'"
```

### Step 4: Update Deployments in E2E Environment

```bash
export KUBECONFIG=~/workspace/hub-of-hubs/test/script/config/clusters

# Update agent on hub1 and hub2 (delete lease and pod together)
for ctx in hub1 hub2; do
  kubectl delete pods -l name=multicluster-global-hub-agent -n multicluster-global-hub-agent --context $ctx && kubectl delete lease multicluster-global-hub-agent-lock -n multicluster-global-hub-agent --context $ctx
done

# Update manager on global-hub (if needed)
kubectl delete pods -l name=multicluster-global-hub-manager -n multicluster-global-hub --context global-hub && kubectl delete lease multicluster-global-hub-manager-lock -n multicluster-global-hub --context global-hub

# Update operator on global-hub (if needed)
kubectl delete pod -n multicluster-global-hub -l name=multicluster-global-hub-operator --context global-hub && kubectl delete lease multicluster-global-hub-operator-lock --context global-hub
```

### Step 5: Verify Pods Are Running

```bash
# Check agent pods
for ctx in hub1 hub2; do
  echo "=== $ctx ==="
  kubectl get pods -n multicluster-global-hub-agent --context $ctx
done

# Check manager/operator pods
echo "=== global-hub ==="
kubectl get pods -n multicluster-global-hub --context global-hub
```

## Quick Reference Commands

### One-liner: Update Agent

```bash
ssh cloud-vm "bash -c 'source ~/.bash_profile 2>/dev/null; export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin; cd ~/workspace/hub-of-hubs && git stash && git pull origin fix-migration-ocm-e2e && export REGISTRY=quay.io/myan && make build-agent-image push-agent-image'" && \
ssh cloud-vm "bash -c 'export KUBECONFIG=~/workspace/hub-of-hubs/test/script/config/clusters; for ctx in hub1 hub2; do kubectl delete lease -n multicluster-global-hub-agent --all --context \$ctx; kubectl delete pod -n multicluster-global-hub-agent --all --context \$ctx; done'"
```

### One-liner: Update Manager

```bash
ssh cloud-vm "bash -c 'source ~/.bash_profile 2>/dev/null; export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin; cd ~/workspace/hub-of-hubs && git stash && git pull origin fix-migration-ocm-e2e && export REGISTRY=quay.io/myan && make build-manager-image push-manager-image'" && \
ssh cloud-vm "bash -c 'export KUBECONFIG=~/workspace/hub-of-hubs/test/script/config/clusters; kubectl delete lease -n multicluster-global-hub --all --context global-hub; kubectl delete pod -n multicluster-global-hub -l name=multicluster-global-hub-manager --context global-hub'"
```

## Notes

- Always delete leases and pods together to avoid pods hanging in pending state
- The e2e branch is `fix-migration-ocm-e2e`
- Images are pushed to `quay.io/myan/`
- Build server is `cloud-vm` with workspace at `~/workspace/hub-of-hubs`


E2E Images

export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE_REF=quay.io/myan/multicluster-global-hub-operator:latest
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE_REF=quay.io/myan/multicluster-global-hub-manager:latest  
export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE_REF=quay.io/myan/multicluster-global-hub-agent:latest  

你可以在 cloud-vm 上使用 make e2e-cleanup 清理环境, 然后使用 make e2e-setup 重新设置环境