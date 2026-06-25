# NetworkPolicy Guide for Multicluster Global Hub

## Overview
This document describes the NetworkPolicy resources deployed in the `multicluster-global-hub` namespace to enhance security by controlling network traffic between components and external systems.

Policies are **operator-managed**: the multicluster-global-hub operator reconciles them when a `MulticlusterGlobalHub` CR is created or updated, and removes them when the CR is deleted. Entry points include `HoHDeployer.deployNetworkPolicy()` and per-component reconcilers.

**Reference:** [ACM-19479](https://redhat.atlassian.net/browse/ACM-19479)

## Components and Ports

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

## Deployed NetworkPolicies

### 1. Default Deny All Policy
Blocks all ingress and egress cluster-wide in the namespace. Component-specific policies below then explicitly allow required traffic.

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

Allows in-cluster clients (manager, grafana, inventory-api, spicedb, etc.) on TCP 5432 via
pod selectors. A separate **ports-only** ingress rule (no `from` selector) also allows TCP 5432
from any source that can reach the pod — including out-of-cluster clients via NodePort.

The ports-only rule is required for Jenkins `globalhub-e2e` BeforeSuite, which creates a
temporary NodePort Service `multicluster-global-hub-postgresql-external` (fixed NodePort
**32433**) and connects from outside the cluster to the hub node IP over TLS
(`test/e2e/suite_test.go`).

> **Follow-up (ACM-31409):** tighten external access with `ipBlock` CIDRs — tracked from
> [PR #2561](https://github.com/stolostron/multicluster-global-hub/pull/2561).

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
  # Allow TCP 5432 from any source (no `from` selector). Required for globalhub-e2e
  # NodePort access (multicluster-global-hub-postgresql-external:32433) and backup tools.
  # Follow-up: scope with ipBlock CIDRs (ACM-31409).
  - ports:
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

Allows in-cluster clients (manager, Strimzi, managed-hub agents) and **external**
TLS bootstrap access via the OpenShift router on TCP 9093. The latter is required
for Jenkins `globalhub-e2e` kafka tests, which connect to `kafka-kafka-tls-bootstrap`
Route (`transport-config` bootstrap.server) from outside the cluster.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kafka
  namespace: multicluster-global-hub
  labels:
    strimzi.io/cluster: kafka
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
      port: 9093
  # Allow Kafka internal communication (between brokers, controllers)
  - from:
    - podSelector:
        matchLabels:
          strimzi.io/cluster: kafka
  # Allow from agents in managed hub namespaces (hosted mode)
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
  # Allow external access via OpenShift router (Kafka TLS bootstrap Route)
  - from:
    - namespaceSelector:
        matchLabels:
          network.openshift.io/policy-group: ingress
    ports:
    - protocol: TCP
      port: 9093
  # Allow metrics scraping from Prometheus (JMX exporter tcp-prometheus)
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-monitoring
    ports:
    - protocol: TCP
      port: 9404
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: openshift-dns
    ports:
    - protocol: UDP
      port: 5353
    - protocol: TCP
      port: 5353
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

## Operator Lifecycle

NetworkPolicies are fully managed by the multicluster-global-hub operator:

- **Create/Update:** The operator reconciles all policies whenever the `MulticlusterGlobalHub` CR is created or updated. No manual `kubectl apply` is required.
- **Delete:** Policies are removed when the `MulticlusterGlobalHub` CR is deleted.
- **Upgrade:** Rolling out a new operator version automatically applies any policy changes.

To verify policies are present after install:

```bash
kubectl get networkpolicies -n multicluster-global-hub
```

To verify pod label selectors are working correctly:

```bash
kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-operator
kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-manager
kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-grafana
kubectl get pods -n multicluster-global-hub -l strimzi.io/cluster=kafka
kubectl get pods -n multicluster-global-hub -l authzed.com/cluster=spicedb
kubectl get pods -n multicluster-global-hub -l app=relations-api
```

If using hosted mode, label managed hub namespaces so the Kafka ingress rule matches agent traffic:

```bash
kubectl label namespace <managed-hub-namespace> global-hub.open-cluster-management.io/managed-hub=true
```

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
- If agents are in remote clusters, Kafka bootstrap Routes need external exposure via the OpenShift router (see Kafka NetworkPolicy ingress on TCP 9093)

### 4. External Kafka Access
If using external Kafka bootstrap servers:
- Add egress rules to allow manager to connect to external Kafka
- Configure appropriate authentication/TLS
- Built-in Strimzi Kafka TLS bootstrap Routes are allowed via `network.openshift.io/policy-group: ingress` on TCP 9093

### 5. Backup Considerations
If using backup solutions (Velero, OADP):
- Add egress rules for manager/postgres to access backup namespace
- The ports-only postgres ingress rule (no `from` selector) allows TCP 5432 from any source;
  backup or restore tools connecting via NodePort or from outside the cluster rely on this rule.
  Prefer explicit namespace selectors for in-cluster backup workloads where possible (see
  PostgreSQL NetworkPolicy; follow-up hardening tracked in ACM-31409).

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

## Known Limitations and Follow-ups

Remaining items are tracked in [ACM-32313](https://redhat.atlassian.net/browse/ACM-32313) (child story of epic [ACM-31409](https://redhat.atlassian.net/browse/ACM-31409)).

- **OVN-Kubernetes enforcement:** Policies are created on KinD (used in product CI) but are only enforced on clusters running OVN-Kubernetes (e.g. OpenShift). Manual soak testing on OVN is tracked in [ACM-32494](https://redhat.atlassian.net/browse/ACM-32494); HoH automation in [ACM-32495](https://redhat.atlassian.net/browse/ACM-32495).
- **Kafka `ipBlock` egress:** Agent egress to external Kafka bootstrap currently uses a broad `namespaceSelector`. Tightening to a specific CIDR `ipBlock` requires the operator to resolve the broker address at runtime — follow-up in [ACM-32313](https://redhat.atlassian.net/browse/ACM-32313).
- **API server `ipBlock`:** The `allow-dns-and-api` policy allows egress to the `default` namespace broadly. Narrowing to the exact API server IP requires runtime address resolution — follow-up in [ACM-32313](https://redhat.atlassian.net/browse/ACM-32313).

## References
- **Jira Ticket**: [ACM-19479](https://redhat.atlassian.net/browse/ACM-19479)
- **Kubernetes NetworkPolicy**: https://kubernetes.io/docs/concepts/services-networking/network-policies/
- **OpenShift NetworkPolicy**: https://docs.openshift.com/container-platform/latest/networking/network_policy/about-network-policy.html
- **Strimzi Kafka Security**: https://strimzi.io/docs/operators/latest/configuring.html#assembly-securing-kafka-str
