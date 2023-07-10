### Create a regional hub cluster (Developer Preview)
Refer to the original [Create cluster](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.8/html/clusters/cluster_mce_overview#creating-a-cluster) document to create the managed cluster in the global hub cluster. Add the label of `global-hub.open-cluster-management.io/hub-cluster-install: ''` to the `managedcluster` custom resource and then the new created managed cluster can automatically be switched to be a regional hub cluster. In other words, the latest version of Red Hat Advanced Cluster Management for Kubernetes is installed in this managed cluster. You can get the Red Hat Advanced Cluster Management hub information in the cluster overview page.

![cluster overview](cluster_overview.png)

### Import a regional hub cluster in hosted mode (Developer Preview)
A regional hub cluster does not require any changes before importing it. The Red Hat Advanced Cluster Management agent is running in a hosting cluster.

1. Import the cluster from the Red Hat Advanced Cluster Management console, add these annotations to the `managedCluster` custom resource. Use the kubeconfig import mode, and disable all add-ons.
```
import.open-cluster-management.io/klusterlet-deploy-mode: Hosted
import.open-cluster-management.io/hosting-cluster-name: local-cluster
addon.open-cluster-management.io/disable-automatic-installation: "true"
```
![import hosted cluster](import_hosted_cluster.png)
Click `Next` to complete the import process.

2. Enable the work-manager addon after the imported cluster is available.
```
oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: work-manager
  namespace: hub1
  annotations:
    addon.open-cluster-management.io/hosting-cluster-name: local-cluster
spec:
  installNamespace: open-cluster-management-hub1-addon-workmanager
EOF
```
You have to create a kubeconfig secret for the work-manager add-on via the following command:
```
oc create secret generic work-manager-managed-kubeconfig --from-file=kubeconfig=<your regional hub kubeconfig> -n open-cluster-management-hub1-addon-workmanager
```
