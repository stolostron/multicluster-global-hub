# Event-Driven Ansible Integration with Global Hub Transport

## Get Started

### Listen for policy events from kafka

1. Start ansible rulebook for policy events

```bash
ansible-rulebook -i inventory.yml --verbose --rulebook kafka-spec-policies.yml
```

2. Create/Delete policies from global hub UI

### Listen for managedcluster events from kafka

1. Start ansible rulebook for managedcluster events

```bash
# export ANSIBLE_DEBUG=true
# export ANSIBLE_VERBOSITY=4
ansible-rulebook -i inventory.yml --verbose --rulebook kafka-status-managedclusters.yml
```

2. Create/Delete managedcluster from regional hub:

```bash
export REGIONAL_HUB_KUBECONFIG=
export MC_NAME=kind-hub1-mc2
oc --kubeconfig=${REGIONAL_HUB_KUBECONFIG} apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: ${MC_NAME}
spec:
  hubAcceptsClient: true
EOF
```

