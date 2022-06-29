## Import a Managed Cluster to the Hypershift Hosted Leaf Hub

1. Set the `KUBECONFIG` environment to access the hypershift hosted cluster:

```bash
export KUBECONFIG=<path-to-hypershift-hosted-cluster-kubeconfig>
```

2. Set the managed cluster name and create managedcluster CR in hypershift hosted cluster:

```bash
export MANAGED_CLUSTER_NAME=mc1
cat << EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: ${MANAGED_CLUSTER_NAME}
spec:
  hubAcceptsClient: true
EOF
```

3. Get the managed cluster import manifests:

```bash
oc get secret -n ${MANAGED_CLUSTER_NAME} ${MANAGED_CLUSTER_NAME}-import -o jsonpath={.data.crds\\\.yaml} | base64 -d  > /tmp/import-crds.yaml
oc get secret -n ${MANAGED_CLUSTER_NAME} ${MANAGED_CLUSTER_NAME}-import -o jsonpath={.data.import\\\.yaml} | base64 -d > /tmp/import.yaml
```

4. Create the import manifests in managed cluster:

```bash
oc --kubeconfig=<path-to-managedcluster-kubeconfig> apply -f /tmp/import-crds.yaml
oc --kubeconfig=<path-to-managedcluster-kubeconfig> apply -f /tmp/import.yaml
```

5. Wait for klusterlet is up and running in managed cluster:

```bash
oc --kubeconfig=<path-to-managedcluster-kubeconfig> -n open-cluster-management-agent get pod
```

6. Check managed cluster status from hosted leaf hub:

```
oc wait --for=condition=ManagedClusterConditionAvailable managedcluster/${MANAGED_CLUSTER_NAME} --timeout=600s
```

7. Apply application addon for managedcluster in hosted leaf hub:

```bash
cat << EOF | oc apply -n ${MANAGED_CLUSTER_NAME} -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: application-manager
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

8. Create policy addon for managedcluster in hosted leaf hub:

```bash
cat << EOF | oc apply -n ${MANAGED_CLUSTER_NAME} -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: config-policy-controller
spec:
  installNamespace: open-cluster-management-agent-addon
---
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: governance-policy-framework
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

9. Check addon status in managed cluster:

```bash
# oc --kubeconfig=<path-to-managedcluster-kubeconfig> -n open-cluster-management-agent-addon get pod
NAME                                           READY   STATUS    RESTARTS   AGE
application-manager-c5659c979-f46zq            1/1     Running   0          95s
config-policy-controller-6d8f74cb5b-sz6w5      1/1     Running   0          27s
governance-policy-framework-768459d787-4bjjr   1/3     Running   0          16s
```
