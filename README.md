# Multicluster Global Hub

[![Build](https://img.shields.io/badge/build-Prow-informational)](https://prow.ci.openshift.org/?repo=stolostron%2F${multicluster-global-hub})
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=open-cluster-management_hub-of-hubs&metric=coverage)](https://sonarcloud.io/dashboard?id=open-cluster-management_hub-of-hubs)
[![Go Reference](https://pkg.go.dev/badge/github.com/stolostron/multicluster-global-hub.svg)](https://pkg.go.dev/github.com/stolostron/multicluster-global-hub)
[![License](https://img.shields.io/github/license/stolostron/multicluster-global-hub)](/LICENSE)

A comprehensive multicluster management solution that enables centralized governance, policy management, and observability across multiple OpenShift/Kubernetes clusters at enterprise scale.

## 🚀 Key Features

- **Centralized Management**: Single control plane for multiple hub clusters
- **Policy Governance**: Global policy distribution and compliance monitoring  
- **Application Lifecycle**: Cross-cluster application deployment and management
- **Migration Support**: Seamless cluster migration between hubs
- **Observability**: Unified dashboards and monitoring across all clusters
- **Event-Driven Architecture**: Real-time synchronization via Apache Kafka

## Conceptual Diagram

![ArchitectureDiagram](doc/architecture/multicluster-global-hub-arch.png)

## 🏗️ Architecture Components

### Global Hub Operator
The root operator that orchestrates the deployment of all Global Hub components. It manages:
- Installation and lifecycle of the Global Hub Manager
- Deployment of Global Hub Agents to managed hubs
- Configuration of underlying infrastructure (PostgreSQL, Kafka, Grafana)

### Global Hub Manager
The central control plane running on the global hub cluster:
- **Data Persistence**: Stores cluster state, policies, and applications in PostgreSQL
- **Event Processing**: Manages Kafka message queues for real-time synchronization
- **API Gateway**: Provides unified APIs for multicluster operations
- **Migration Controller**: Orchestrates seamless cluster migrations between hubs

### Global Hub Agent
Lightweight agents deployed on each managed hub cluster:
- **Bidirectional Sync**: Synchronizes cluster data upstream and policies/apps downstream
- **Local Cache**: Maintains local state for improved performance and resilience
- **Event Publishing**: Publishes cluster events and status updates via Kafka
- **Migration Support**: Handles cluster migration operations

### Observability Stack
Comprehensive monitoring and visualization:
- **Grafana Dashboards**: Pre-configured dashboards for multicluster metrics
- **PostgreSQL Integration**: Uses collected data as the primary data source
- **Route Exposure**: Accessible via OpenShift routes for seamless integration

## 📋 Prerequisites

- **OpenShift 4.10+** or **Kubernetes 1.24+**
- **Advanced Cluster Management (ACM) 2.6+** or **Open Cluster Management (OCM)**
- **Cluster Admin** privileges on the global hub cluster
- **PostgreSQL 13+** (can be auto-provisioned)
- **Apache Kafka** (can be auto-provisioned)

## 🚀 Installation

### Option 1: OperatorHub (Recommended)

1. **OpenShift Console**: Navigate to Operators → OperatorHub
2. **Search**: Look for "Multicluster Global Hub"
3. **Install**: Select the operator from Community Operators
4. **Create Instance**: Apply a MulticlusterGlobalHub custom resource

### Option 2: Development Installation

```bash
# Clone the repository
git clone git@github.com:stolostron/multicluster-global-hub.git
cd multicluster-global-hub/operator

# Build and push custom image
export IMG=<your-registry>/multicluster-global-hub-operator:<tag>
make docker-build docker-push IMG=$IMG

# Deploy the operator
make deploy IMG=$IMG

# Create MulticlusterGlobalHub instance
kubectl apply -k config/samples/
```

### Option 3: Helm Chart

```bash
# Add the helm repository
helm repo add multicluster-global-hub https://stolostron.github.io/multicluster-global-hub
helm repo update

# Install with custom values
helm install global-hub multicluster-global-hub/multicluster-global-hub \
  --namespace multicluster-global-hub \
  --create-namespace \
  --values values.yaml
```

## 🔧 Configuration

### Basic Configuration

```yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
spec:
  dataLayer:
    postgres:
      retention: "18m"
    kafka:
      topics:
        replicas: 3
        partitions: 10
  advanced:
    imagePullPolicy: Always
    nodeSelector:
      node-type: infra
```

### Production Configuration

```yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
spec:
  enableLocalPolicies: true
  dataLayer:
    postgres:
      retention: "30d"
      resources:
        requests:
          memory: "2Gi"
          cpu: "500m"
    kafka:
      topics:
        replicas: 3
        partitions: 20
      resources:
        requests:
          memory: "4Gi"
          cpu: "1000m"
  advanced:
    nodeSelector:
      node-type: infra
    tolerations:
    - key: "infra"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"
```

## 🗑️ Uninstallation

```bash
# Remove all MulticlusterGlobalHub instances
kubectl delete mgh --all

# Remove the operator (development installations)
make undeploy

# For OperatorHub installations, uninstall via OpenShift Console
```

## 💡 Usage Examples

### Managing Policies Across Clusters

```bash
# Create a global policy
cat << EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: namespace-policy
  namespace: multicluster-global-hub
spec:
  remediationAction: enforce
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: production
EOF
```

### Cluster Migration

```bash
# Create a cluster migration
cat << EOF | kubectl apply -f -
apiVersion: migration.open-cluster-management.io/v1alpha1
kind: ManagedClusterMigration
metadata:
  name: migrate-cluster-1
  namespace: multicluster-global-hub
spec:
  sourceHub: hub-1
  targetHub: hub-2
  clusterName: managed-cluster-1
EOF
```

### Application Deployment

```bash
# Deploy an application across multiple clusters
cat << EOF | kubectl apply -f -
apiVersion: app.k8s.io/v1beta1
kind: Application
metadata:
  name: nginx-app
  namespace: multicluster-global-hub
spec:
  componentKinds:
  - group: apps
    kind: Deployment
  descriptor:
    type: nginx
  selector:
    matchLabels:
      app: nginx
EOF
```

## 🔍 Monitoring and Observability

### Access Grafana Dashboards

```bash
# Get Grafana route URL
oc get route multicluster-global-hub-grafana -n multicluster-global-hub

# Get admin password
oc get secret multicluster-global-hub-grafana -n multicluster-global-hub \
  -o jsonpath='{.data.admin-password}' | base64 -d
```

### Key Metrics to Monitor

- **Cluster Health**: Overall status of managed clusters
- **Policy Compliance**: Policy violation trends across clusters  
- **Application Status**: Deployment success rates and health
- **Migration Progress**: Active migrations and completion status
- **Resource Utilization**: CPU, memory, and storage usage patterns

## 🔧 Troubleshooting

### Common Issues

#### Operator Installation Fails
```bash
# Check operator logs
kubectl logs -n multicluster-global-hub deployment/multicluster-global-hub-operator

# Verify prerequisites
kubectl get crd managedclusters.cluster.open-cluster-management.io
```

#### Agent Connection Issues
```bash
# Check agent status on managed hub
kubectl get pods -n multicluster-global-hub-agent

# Verify Kafka connectivity
kubectl exec -n multicluster-global-hub deployment/multicluster-global-hub-manager -- \
  kafka-console-consumer --bootstrap-server localhost:9092 --list
```

#### Migration Stuck
```bash
# Check migration status
kubectl get managedclustermigration -A

# View migration events
kubectl describe managedclustermigration <migration-name>
```

### Debug Commands

```bash
# Collect support bundle
kubectl adm inspect clusterscoped/multicluster-global-hub

# Check resource quotas
kubectl describe resourcequota -n multicluster-global-hub

# Verify network policies
kubectl get networkpolicies -n multicluster-global-hub
```

## 🤝 Contributing

Go to the [Contributing guide](CONTRIBUTING.md) to learn how to get involved.

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🔗 Related Projects

- [Open Cluster Management](https://open-cluster-management.io/)
- [Advanced Cluster Management](https://access.redhat.com/products/red-hat-advanced-cluster-management)
- [Submariner](https://submariner.io/) - Multi-cluster networking
- [Argo CD](https://argoproj.github.io/argo-cd/) - GitOps continuous delivery
