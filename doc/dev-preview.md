### Create a regional hub cluster (dev preview)
Refer to the original [Create cluster](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.6/html/multicluster_engine/multicluster_engine_overview#creating-a-cluster) document to create the managed cluster in the global hub cluster. add labels of `global-hub.open-cluster-management.io/hub-cluster-install: ''` in managedcluster CR and then the new created managed cluster can be switched to be a regional hub cluster automatically. In other words, the latest released RHACM is installed in this managed cluster. You can get the ACM hub information in the cluster overview page.
![cluster overview](cluster_overview.png)
### Create a global policy (dev preview)
You can navigate to `Governance` from the navigation menu, and then click the `Create policy` to create a global policy. The `policyset` is not supported yet.
![create global policy](create_policy.png)
If you want to create a global policy which can be propagated to regional hub clusters by transport, you need to add `global-hub.open-cluster-management.io/global-resource=""` manually for the Policy/PlacementRule/PlacementBinding.
### Create a global application (dev preview)
You can navigate to `Applications` from the navigation menu, and then click the `Create application` to select `Subscription`. The `ApplicationSet` is not supported yet.
If you want to create a global application which can be propagated to regional hub clusters by transport, you need to add `global-hub.open-cluster-management.io/global-resource=""` manually for the Application/PlacementRule/Subscription.
### Access the global resources via multicluster global hub APIs (dev preview)
Multicluster global hub APIs contains three resource categories: managed clusters, policies, application subscriptions. Each type of resource has two possible requests: list, get.

<strong>Prerequisites:</strong>
- You need log into the multicluster global hub cluster and retrieve the access token by running

    ```bash
    export TOKEN=$(oc whoami -t)
    ```
    to access the multicluster global hub APIs.

- Get the host of multicluster global hub API

```bash
export GLOBAL_HUB_API_HOST=$(oc -n open-cluster-management get route multicluster-global-hub-manager -o jsonpath={.spec.host})
```

1. List managed clusters:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?labelSelector=env%3Dproduction&limit=2"
```

2. Patch label for managed cluster:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" -X PATCH "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedcluster/<managed_cluster_uid>" -d '[{"op":"add","path":"/metadata/labels/foo","value":"bar"}]'
```

3. List policies:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?labelSelector=env%3Dproduction&limit=2"
```

4. Get policy status with policy ID:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policy/<policy_uid>/status"
```

5. List subscriptions:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?labelSelector=env%3Dproduction&limit=2"
```

6. Get subscription report with subscription ID:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptionreport/<sub_uid>"
```
