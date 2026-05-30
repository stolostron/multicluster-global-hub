# NetworkPolicy Proposal for Multicluster Global Hub

## Overview
This proposal introduces NetworkPolicy resources for the `multicluster-global-hub` namespace to enhance security by controlling network traffic between components and external systems.

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
   - Various Kafka ports

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

### 2. Allow DNS Resolution (All Pods)
All pods need DNS resolution.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns
  namespace: multicluster-global-hub
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
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

## Implementation Strategy

### Phase 1: Preparation
1. **Verify Labels**: Ensure all pods have correct labels
   ```bash
   # Check operator labels
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-operator
   
   # Check manager labels
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-manager
   
   # Check grafana labels
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-grafana
   
   # Check postgres labels
   kubectl get pods -n multicluster-global-hub -l name=multicluster-global-hub-postgres
   
   # Check kafka labels
   kubectl get pods -n multicluster-global-hub -l strimzi.io/cluster=kafka
   ```

2. **Label Managed Hub Namespaces** (if using hosted mode for agents):
   ```bash
   kubectl label namespace <managed-hub-namespace> global-hub.open-cluster-management.io/managed-hub=true
   ```

### Phase 2: Testing in Development
1. Deploy NetworkPolicies in a test environment
2. Verify all components can communicate:
   - Manager → PostgreSQL
   - Manager → Kafka
   - Grafana → PostgreSQL
   - Grafana route accessibility
   - Webhook functionality
   - Metrics scraping
3. Monitor logs for connection failures

### Phase 3: Iterative Refinement
1. Monitor network traffic patterns
2. Adjust policies based on observed traffic
3. Test agent connectivity from managed hubs
4. Document any additional required traffic

### Phase 4: Production Rollout
1. Apply policies during maintenance window
2. Monitor for issues
3. Have rollback plan ready (delete NetworkPolicies if issues arise)

## File Structure

Create the following directory structure:

```
operator/config/network-policies/
├── kustomization.yaml
├── default-deny-all.yaml
├── allow-dns.yaml
├── operator-netpol.yaml
├── manager-netpol.yaml
├── grafana-netpol.yaml
├── postgres-netpol.yaml
└── kafka-netpol.yaml
```

**kustomization.yaml:**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: multicluster-global-hub

resources:
- default-deny-all.yaml
- allow-dns.yaml
- operator-netpol.yaml
- manager-netpol.yaml
- grafana-netpol.yaml
- postgres-netpol.yaml
- kafka-netpol.yaml
```

## Deployment

```bash
# Deploy NetworkPolicies
kubectl apply -k operator/config/network-policies/

# Or include in the main deployment
# Add to operator/config/default/kustomization.yaml:
# - ../network-policies
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
# Apply NetworkPolicies
kubectl apply -k operator/config/network-policies/

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

# Monitor network policy logs (OpenShift)
kubectl logs -n openshift-sdn -l app=sdn 2>&1 | grep -i "denied"
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

1. Review and approve this proposal
2. Create the NetworkPolicy manifests in the operator codebase
3. Test in development environment
4. Update operator deployment to include NetworkPolicies
5. Document in operator README
6. Update E2E tests to verify NetworkPolicies don't break functionality

## References
- **Jira Ticket**: [ACM-19479](https://redhat.atlassian.net/browse/ACM-19479)
- **Kubernetes NetworkPolicy**: https://kubernetes.io/docs/concepts/services-networking/network-policies/
- **OpenShift NetworkPolicy**: https://docs.openshift.com/container-platform/latest/networking/network_policy/about-network-policy.html
- **Strimzi Kafka Security**: https://strimzi.io/docs/operators/latest/configuring.html#assembly-securing-kafka-str
