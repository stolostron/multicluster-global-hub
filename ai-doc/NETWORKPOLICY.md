# NetworkPolicy Guide for Multicluster Global Hub

## Overview
This document describes the NetworkPolicy resources deployed in the `multicluster-global-hub` namespace to enhance security by controlling network traffic between components and external systems.

Policies are **operator-managed**: the multicluster-global-hub operator reconciles them when a `MulticlusterGlobalHub` CR is created or updated, and removes them when the CR is deleted. Entry points include `HoHDeployer.deployNetworkPolicy()` and per-component reconcilers.

**Reference:** [ACM-19479](https://redhat.atlassian.net/browse/ACM-19479)

## Architecture Analysis

### Components in multicluster-global-hub namespace:

1. **multicluster-global-hub-operator**
   - Webhook server: Port 9443
   - Metrics: Port 8081
   - Needs: Kubernetes API access

2. **multicluster-global-hub-manager**
   - HTTP API server: Port 8080
   - Metrics: Port 8384
   - Needs: PostgreSQL (5432), Kafka, Kubernetes API access

3. **multicluster-global-hub-grafana**
   - Grafana: Port 3001 (internal)
   - Grafana alerts: Port 9094
   - OAuth proxy: Port 9443 (external)
   - Needs: PostgreSQL (5432), Kubernetes API

4. **PostgreSQL StatefulSet**
   - Database: Port 5432
   - Prometheus exporter: Port 9187

5. **Kafka (Strimzi)**
   - Bootstrap: Port 9092/9093
   - JMX Prometheus metrics: Port 9404 (`tcp-prometheus`)
   - Needs: Internal cluster communication, Kubernetes API

6. **Inventory API**
   - HTTP/gRPC: Ports 8081, 9081
   - Needs: PostgreSQL (5432), Kafka (9093), SpiceDB, relations-api, Kubernetes API

7. **Relations API**
   - HTTP/gRPC: Ports 8000, 9000
   - Needs: SpiceDB (50051), Kubernetes API

8. **SpiceDB**
   - gRPC: 50051; HTTP gateway: 8443; metrics: 9090; dispatch: 50053
   - Needs: PostgreSQL (5432), Kubernetes API

9. **Strimzi Cluster Operator**
   - Needs: Kafka pods (9090/9091/8443/9093), DNS, Kubernetes API

10. **Agent (local and addon deployments)**
    - Egress-only policy
    - Needs: DNS, Kubernetes API (443), Kafka (9092–9095), webhooks (8443)

## Proposed NetworkPolicies

### 1. Default Deny All Policy
Start with a default deny policy, then explicitly allow required traffic.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: multicluster-global-hub
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
```

### 2. Allow DNS and Kubernetes API Access (All Pods)
All pods need DNS resolution and Kubernetes API access. The deployed policy is named `allow-dns-and-api`.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns-and-api
  namespace: multicluster-global-hub
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  # Allow access to Kubernetes API server
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
    - protocol: TCP
      port: 6443
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-dns
    ports:
    - protocol: UDP
      port: 5353
    - protocol: TCP
      port: 5353
  # Also allow kube-dns for non-OpenShift clusters
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
    podSelector:
      matchLabels:
        k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53
```

### 3. Operator NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: multicluster-global-hub-operator
  namespace: multicluster-global-hub
spec:
  podSelector:
    matchLabels:
      name: multicluster-global-hub-operator
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow webhook traffic from K8s API server
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 9443
  # Allow metrics scraping from Prometheus
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
    ports:
    - protocol: TCP
      port: 8081
  egress:
  # Allow access to Kubernetes API server
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
  # Allow access to manage resources in the same namespace
  - to:
    - podSelector: {}
  # Allow access to other namespaces for operator operations (e.g., managed hub namespaces)
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443
```

### 4. Manager NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: multicluster-global-hub-manager
  namespace: multicluster-global-hub
spec:
  podSelector:
    matchLabels:
      name: multicluster-global-hub-manager
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow API server access from within cluster
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 8080
  # Allow metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
    ports:
    - protocol: TCP
      port: 8384
  egress:
  # Allow access to PostgreSQL
  - to:
    - podSelector:
        matchLabels:
          name: multicluster-global-hub-postgres
    ports:
    - protocol: TCP
      port: 5432
  # Allow access to Kafka
  - to:
    - podSelector:
        matchLabels:
          strimzi.io/cluster: kafka
    ports:
    - protocol: TCP
      port: 9092
    - protocol: TCP
      port: 9093
    - protocol: TCP
      port: 9094
  # Allow access to Kubernetes API
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
  # Allow access to other namespaces (for managing resources)
  - to:
    - namespaceSelector: {}
```

### 5. Grafana NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: multicluster-global-hub-grafana
  namespace: multicluster-global-hub
spec:
  podSelector:
    matchLabels:
      name: multicluster-global-hub-grafana
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow external access via OpenShift router
  - from:
    - namespaceSelector:
        matchLabels:
          network.openshift.io/policy-group: ingress
    ports:
    - protocol: TCP
      port: 9443
  # Allow metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
    ports:
    - protocol: TCP
      port: 3001
    - protocol: TCP
      port: 9443
  egress:
  # Allow access to PostgreSQL
  - to:
    - podSelector:
        matchLabels:
          name: multicluster-global-hub-postgres
    ports:
    - protocol: TCP
      port: 5432
  # Allow OAuth token validation with OpenShift OAuth
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-authentication
  # Allow access to Kubernetes API for OAuth
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
  # Allow HTTPS to external services for grafana proxy
  - ports:
    - protocol: TCP
      port: 443
```

### 6. PostgreSQL NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: multicluster-global-hub-postgres
  namespace: multicluster-global-hub
spec:
  podSelector:
    matchLabels:
      name: multicluster-global-hub-postgres
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow connections from manager
  - from:
    - podSelector:
        matchLabels:
          name: multicluster-global-hub-manager
    ports:
    - protocol: TCP
      port: 5432
  # Allow connections from grafana
  - from:
    - podSelector:
        matchLabels:
          name: multicluster-global-hub-grafana
    ports:
    - protocol: TCP
      port: 5432
  # Allow Prometheus exporter scraping
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
    ports:
    - protocol: TCP
      port: 9187
    - protocol: TCP
      port: 80
  egress:
  # PostgreSQL typically doesn't need egress, but allow for certificate verification
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
```

### 7. Kafka NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kafka
  namespace: multicluster-global-hub
spec:
  podSelector:
    matchLabels:
      strimzi.io/cluster: kafka
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow connections from manager
  - from:
    - podSelector:
        matchLabels:
          name: multicluster-global-hub-manager
    ports:
    - protocol: TCP
      port: 9092
    - protocol: TCP
      port: 9093
    - protocol: TCP
      port: 9094
  # Allow Kafka internal communication (between brokers, ZooKeeper, etc.)
  - from:
    - podSelector:
        matchLabels:
          strimzi.io/cluster: kafka
  # Allow from agents in managed hub namespaces
  - from:
    - namespaceSelector:
        matchLabels:
          global-hub.open-cluster-management.io/managed-hub: "true"
    podSelector:
      matchLabels:
        name: multicluster-global-hub-agent
    ports:
    - protocol: TCP
      port: 9093
  # Allow metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
  egress:
  # Allow Kafka internal communication
  - to:
    - podSelector:
        matchLabels:
          strimzi.io/cluster: kafka
  # Allow Kafka to access Kubernetes API (for Strimzi operator coordination)
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: default
    ports:
    - protocol: TCP
      port: 443
```

### 8. Inventory API NetworkPolicy

Managed by the inventory reconciler. See `operator/pkg/controllers/inventory/manifests/inventory-api/networkpolicy.yaml`.

- **Ingress:** TCP 8081, 9081
- **Egress:** PostgreSQL (5432), Kafka (9093), SpiceDB, relations-api, Kubernetes API (443)

### 9. Relations API NetworkPolicy

Managed by the inventory reconciler. See `operator/pkg/controllers/inventory/manifests/relations-api/networkpolicy.yaml`.

- **Ingress:** TCP 8000, 9000
- **Egress:** SpiceDB (50051), Kubernetes API and other dependencies

### 10. SpiceDB NetworkPolicy

Managed by the SpiceDB reconciler. See `operator/pkg/controllers/inventory/manifests/spicedb-operator/spicedb-networkpolicy.yaml`.

- **Ingress:** TCP 50051 (gRPC), 8443 (HTTP), 9090 (metrics), 50053 (dispatch)
- **Egress:** PostgreSQL (5432), Kubernetes API

### 11. Strimzi Cluster Operator NetworkPolicy

Managed by the transporter reconciler. See `operator/pkg/controllers/transporter/protocol/manifests/strimzi-cluster-operator-networkpolicy.yaml`.

- **Egress-only:** DNS, Kafka pods (9090/9091/8443/9093), Kubernetes API

### 12. Agent NetworkPolicy (local and addon)

Managed by the agent controllers. Manifests:

- `operator/pkg/controllers/agent/manifests/networkpolicy.yaml` (local agent)
- `operator/pkg/controllers/agent/addon/manifests/templates/agent/multicluster-global-hub-agent-networkpolicy.yaml` (addon)

- **Egress-only:** DNS, Kubernetes API (443), in-cluster Kafka (9092–9095), external Kafka/bootstrap (9092/9093), webhooks (8443)

## Implementation Strategy

**Note:** NetworkPolicies are managed automatically by the multicluster-global-hub operator. The operator reconciles policies when the `MulticlusterGlobalHub` CR is created or updated, and removes them when the CR is deleted. The phases below describe **validation and testing** — not manual `kubectl apply` deployment.

### Phase 1: Preparation
1. **Verify labels** on pods in the MGH namespace:
   ```bash
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-operator
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-manager
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-grafana
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-postgres
   kubectl get pods -n multicluster-global-hub -l strimzi.io/cluster=kafka
   kubectl get pods -n multicluster-global-hub -l authzed.com/cluster=spicedb
   kubectl get pods -n multicluster-global-hub -l app=relations-api
   ```

2. **Label managed hub namespaces** (if using hosted mode for agents):
   ```bash
   kubectl label namespace <managed-hub-namespace> global-hub.open-cluster-management.io/managed-hub=true
   ```

3. **Confirm operator reconciliation** — policies should appear after MGH install:
   ```bash
   kubectl get networkpolicies -n multicluster-global-hub
   ```

### Phase 2: Testing in Development
1. Install or upgrade Global Hub and confirm the operator creates NetworkPolicies
2. Verify component connectivity:
   - Manager → PostgreSQL, Kafka
   - Grafana → PostgreSQL, OAuth route
   - Inventory → PostgreSQL, Kafka, SpiceDB
   - Agent → Kafka, Kubernetes API
   - Webhook and metrics scraping
3. Monitor logs for connection failures

### Phase 3: Iterative Refinement
1. Monitor network traffic patterns on OpenShift (OVN-Kubernetes)
2. Adjust policy manifests in component controllers as needed
3. Test agent connectivity from managed hubs
4. Document any additional required traffic

### Phase 4: Production Rollout
1. Roll out via operator upgrade during a maintenance window
2. Monitor for issues
3. Rollback: delete the MGH CR or remove NetworkPolicy RBAC to stop reconciliation; delete policies manually if needed

## File Structure

NetworkPolicy YAML is **co-located with each component's controller manifests**, not in a single central directory:

```text
operator/pkg/controllers/
├── networkpolicy/manifests/           # Shared baseline policies
│   ├── default-deny-all.yaml
│   ├── allow-dns-and-api.yaml
│   └── operator-networkpolicy.yaml
├── manager/manifests/networkpolicy.yaml
├── grafana/manifests/networkpolicy.yaml
├── storage/manifests.sts/postgres-networkpolicy.yaml
├── transporter/protocol/manifests/
│   ├── kafka-networkpolicy.yaml
│   └── strimzi-cluster-operator-networkpolicy.yaml
├── inventory/manifests/
│   ├── inventory-api/networkpolicy.yaml
│   ├── relations-api/networkpolicy.yaml
│   └── spicedb-operator/spicedb-networkpolicy.yaml
└── agent/
    ├── manifests/networkpolicy.yaml                          # local agent
    └── addon/manifests/templates/agent/
        └── multicluster-global-hub-agent-networkpolicy.yaml  # addon agent
```

Controllers embed these manifests via `//go:embed` and reconcile them into the MGH namespace.

## Deployment

NetworkPolicies are deployed automatically by the operator. No standalone `kubectl apply -k` step is required.

To verify after install:

```bash
kubectl get networkpolicies -n multicluster-global-hub
kubectl describe networkpolicy -n multicluster-global-hub default-deny-all
```

## Important Considerations

### 1. Label Requirements
Ensure the operator deployment manifests apply these labels correctly:
- **Operator pods**: `name: multicluster-global-hub-operator`
- **Manager pods**: `name: multicluster-global-hub-manager`
- **Grafana pods**: `name: multicluster-global-hub-grafana`
- **PostgreSQL pods**: `name: multicluster-global-hub-postgres`
- **Kafka pods**: `strimzi.io/cluster: kafka` (set by Strimzi)

### 2. OpenShift vs Kubernetes
The policies assume OpenShift. For vanilla Kubernetes:
- Replace `openshift-monitoring` with your monitoring namespace
- Replace `network.openshift.io/policy-group: ingress` with your ingress controller namespace
- Replace `openshift-dns` with `kube-system` for DNS

### 3. Managed Hub Agents
For agents in managed hub clusters:
- If agents are in the same cluster (hosted mode), label their namespaces with:
  ```
  global-hub.open-cluster-management.io/managed-hub: "true"
  ```
- If agents are in remote clusters, Kafka needs external exposure (not covered by these policies)

### 4. External Kafka Access
If using external Kafka bootstrap servers:
- Add egress rules to allow manager to connect to external Kafka
- Configure appropriate authentication/TLS

### 5. Backup Considerations
If using backup solutions (Velero, OADP):
- Add egress rules for manager/postgres to access backup namespace
- Allow backup namespace ingress to postgres for data backup

## Testing Commands

```bash
# Confirm operator created NetworkPolicies
kubectl get networkpolicies -n multicluster-global-hub

# Test manager to postgres connectivity
kubectl exec -n multicluster-global-hub deployment/multicluster-global-hub-manager -- \
  pg_isready -h multicluster-global-hub-postgres -p 5432

# Test manager to kafka connectivity
kubectl exec -n multicluster-global-hub deployment/multicluster-global-hub-manager -- \
  nc -zv kafka-kafka-bootstrap 9092

# Test Grafana route access
GRAFANA_ROUTE=$(kubectl get route -n multicluster-global-hub multicluster-global-hub-grafana -o jsonpath='{.spec.host}')
curl -k -I https://$GRAFANA_ROUTE

# Test webhook
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations

# Check for denied connections in OVN logs
kubectl logs -n openshift-ovn-kubernetes -l app=ovnkube-node --tail=100 | grep -i "drop\|reject"
```

## Monitoring for Issues

```bash
# Check manager logs for connection issues
kubectl logs -n multicluster-global-hub deployment/multicluster-global-hub-manager | \
  grep -i "connection refused\|timeout\|dial tcp"

# Check grafana logs
kubectl logs -n multicluster-global-hub deployment/multicluster-global-hub-grafana | \
  grep -i "connection refused\|timeout"

# Check operator logs
kubectl logs -n multicluster-global-hub deployment/multicluster-global-hub-operator | \
  grep -i "connection refused\|timeout"

# Monitor network policy drops (OpenShift OVN-Kubernetes)
kubectl logs -n openshift-ovn-kubernetes -l app=ovnkube-node --tail=100 | grep -i "drop\|reject"
```

## Rollback Plan

If issues occur:

```bash
# Quick rollback - delete all NetworkPolicies
kubectl delete networkpolicies -n multicluster-global-hub --all

# Or delete specific policy
kubectl delete networkpolicy -n multicluster-global-hub <policy-name>
```

## Security Benefits

1. **Least Privilege**: Each component can only communicate with necessary services
2. **Defense in Depth**: Even if a pod is compromised, network access is restricted
3. **Compliance**: Meets security requirements for network segmentation
4. **Blast Radius Reduction**: Limits lateral movement in case of security breach

## Next Steps

1. Merge NetworkPolicy support ([ACM-19479](https://redhat.atlassian.net/browse/ACM-19479))
2. Validate on OpenShift clusters with OVN-Kubernetes (policies are created on KinD but not enforced)
3. Phase 2 hardening: tighten ingress/egress selectors per component (tracked separately)
4. Extend E2E coverage for NetworkPolicy enforcement on OpenShift

## References
- **Jira Ticket**: [ACM-19479](https://redhat.atlassian.net/browse/ACM-19479)
- **Kubernetes NetworkPolicy**: https://kubernetes.io/docs/concepts/services-networking/network-policies/
- **OpenShift NetworkPolicy**: https://docs.openshift.com/container-platform/latest/networking/network_policy/about-network-policy.html
- **Strimzi Kafka Security**: https://strimzi.io/docs/operators/latest/configuring.html#assembly-securing-kafka-str
