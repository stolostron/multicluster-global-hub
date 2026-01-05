---
argument-hint: <global-hub-kubeconfig> [managed-hub-kubeconfig...]
description: Watch Global Hub manager and agent logs with debug level, supporting multiple hubs
allowed-tools: [Bash, Read, AskUserQuestion]
---

Watch and collect debug logs from Global Hub manager and agents across multiple hub clusters. Automatically detects running pods, sets debug log level, and streams logs to local files.

## Implementation Steps

1. **Detect Global Hub Namespace**: Query the MulticlusterGlobalHub CR to determine the namespace (usually `multicluster-global-hub`)

2. **Configure Debug Logging**: Create or update `multicluster-global-hub-config` ConfigMap with `logLevel: debug` in the detected namespace

3. **Identify Active Pods**:
   - Find the leader manager pod using the `multicluster-global-hub-manager-lock` lease
   - Locate the global hub agent pod in the same namespace (if exists)
   - For each managed hub kubeconfig, find the agent pod in `multicluster-global-hub-agent` namespace

4. **Start Log Streaming**: Launch background processes to stream logs to local files:
   - `global-hub-manager.log` - Manager leader pod logs
   - `global-hub-agent.log` - Global hub agent logs (if exists)
   - `<hub-name>-agent.log` - Each managed hub agent logs

5. **Verify Log Collection**: Check that all log files are being written and debug level is active

## Usage Examples

- `/debug/watch-logs /root/hub1/kubeconfig` - Watch only global hub logs
- `/debug/watch-logs /root/hub1/kubeconfig /root/hub2/kubeconfig` - Watch global hub and one managed hub
- `/debug/watch-logs $GLOBAL_HUB_KC $MANAGED_HUB1_KC $MANAGED_HUB2_KC` - Watch multiple hubs

## Notes

- Logs are written to current directory
- Global hub namespace is auto-detected from MulticlusterGlobalHub CR
- Global hub agent may not exist on the global hub cluster itself
- First kubeconfig is always treated as global hub
- Subsequent kubeconfigs are treated as managed hubs
- Manager leader is identified via `multicluster-global-hub-manager-lock` lease
- Agent pods on managed hubs are in `multicluster-global-hub-agent` namespace
- Background processes continue until killed - use `/tasks` to manage them
- ConfigMap `multicluster-global-hub-config` is created if it doesn't exist
