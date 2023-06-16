### Create a regional hub cluster (dev preview)
Refer to the original [Create cluster](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.6/html/multicluster_engine/multicluster_engine_overview#creating-a-cluster) document to create the managed cluster in the global hub cluster. add labels of `global-hub.open-cluster-management.io/hub-cluster-install: ''` in managedcluster CR and then the new created managed cluster can be switched to be a regional hub cluster automatically. In other words, the latest released RHACM is installed in this managed cluster. You can get the ACM hub information in the cluster overview page.
![cluster overview](cluster_overview.png)
### Import a regional hub cluster in hosted mode (dev preview)
It does not require any changes before importing it. The ACM agent is running in a hosting cluster.
1. Import the cluster from the ACM console, add these annotations to the managedCluster, use the kubeconfig import mode, and disable all add-ons.
```
import.open-cluster-management.io/klusterlet-deploy-mode: Hosted
import.open-cluster-management.io/hosting-cluster-name: local-cluster
addon.open-cluster-management.io/disable-automatic-installation: "true"
```
![import hosted cluster](import_hosted_cluster.png)
Click `Next` Button to complete the import process.

2. Enable work-manager addon after the imported cluster is available.
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
