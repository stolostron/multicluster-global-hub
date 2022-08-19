## Import Managed Cluster to the Hypershift Hosted Regional Hub

1. Set the following environment variables to access Hypershift hosted cluster and managed cluster:

```bash
export HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG=<kubeconfig-path-to-hypershift-hosted-cluster>
export MANAGED_CLUSTER_NAME=<managed-cluster-name>
export MANAGED_CLUSTER_KUBECONFIG=<kubeconfig-path-to-hypershift-hosted-cluster>
```

2. Set the managed cluster name and create managedcluster CR in Hypershift hosted cluster:

```bash
oc --kubeconfig=${HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG} apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: ${MANAGED_CLUSTER_NAME}
spec:
  hubAcceptsClient: true
EOF
```

3. Retrieve the manifests used to import managed cluster from Hypershift hosted cluster:

```bash
oc --kubeconfig=${HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG} get secret \
  -n ${MANAGED_CLUSTER_NAME} ${MANAGED_CLUSTER_NAME}-import \
  -o jsonpath={.data.crds\\\.yaml} | base64 -d  > /tmp/import-crds.yaml
oc --kubeconfig=${HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG} get secret \
  -n ${MANAGED_CLUSTER_NAME} ${MANAGED_CLUSTER_NAME}-import \
  -o jsonpath={.data.import\\\.yaml} | base64 -d > /tmp/import.yaml
```

4. Apply the manifests retrieved in the last step to managed cluster:

```bash
oc --kubeconfig=${MANAGED_CLUSTER_KUBECONFIG} apply -f /tmp/import-crds.yaml
oc --kubeconfig=${MANAGED_CLUSTER_KUBECONFIG} apply -f /tmp/import.yaml
```

5. Wait for klusterlet is up and running in managed cluster:

```bash
oc --kubeconfig=${MANAGED_CLUSTER_KUBECONFIG} -n open-cluster-management-agent get pod
```

6. Check managed cluster status from Hypershift hosted cluster:

```bash
oc --kubeconfig=${HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG} wait --for=condition=ManagedClusterConditionAvailable \
  managedcluster/${MANAGED_CLUSTER_NAME} --timeout=600s
```

7. Apply application and policy addons for managedcluster from Hypershift hosted cluster:

```bash
oc --kubeconfig=${HYPERSHIFT_HOSTED_CLUSTER_KUBECONFIG} apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: application-manager
  namespace: ${MANAGED_CLUSTER_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
---
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: config-policy-controller
  namespace: ${MANAGED_CLUSTER_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
---
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: governance-policy-framework
  namespace: ${MANAGED_CLUSTER_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

8. Check addon status from managed cluster:

```bash
# oc --kubeconfig=${MANAGED_CLUSTER_KUBECONFIG} -n open-cluster-management-agent-addon get pod
NAME                                           READY   STATUS    RESTARTS   AGE
application-manager-c5659c979-f46zq            1/1     Running   0          95s
config-policy-controller-6d8f74cb5b-sz6w5      1/1     Running   0          27s
governance-policy-framework-768459d787-4bjjr   1/3     Running   0          16s
```
