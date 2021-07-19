# The Hub-of-Hubs demo script

1. The user defines a policy, a placement rule and a placement rule binding in their namespace, by ACM UI or kubectl.
1. The user observes policy compliance status in the UI of hub-of-hubs and of leaf hubs.
1. The user observes managed clusters in the database.

## Prequisites

* Three clusters with ACM Hubs on them, `hoh` (the hub of hubs), `hub1` and `hub2`.
* Environment variables hold kubernetes configurations of the clusters,
`TOP_HUB_CONFIG`, `HUB1_CONFIG` and `HUB2_CONFIG`.
* Leaf hub components installed on `hub1` and `hub2`, hub-of-hubs spec and status sync controllers run on `hoh` cluster.
* Tranport bridge and transport server-side run on `hoh` or elsewhere.
* The database can be accessed for querying/deleting

### Before the demo

1.  Scale to 0 leaf hub controllers on `hub1` and `hub2`:

    ```
    $ kubectl scale deployment -n open-cluster-management --kubeconfig $HUB1_CONFIG leaf-hub-spec-sync leaf-hub-status-sync --replicas 0
    deployment.apps/leaf-hub-spec-sync scaled
    deployment.apps/leaf-hub-status-sync scaled
    ```

    ```
    $ kubectl scale deployment -n open-cluster-management --kubeconfig $HUB2_CONFIG leaf-hub-spec-sync leaf-hub-status-sync --replicas 0
    deployment.apps/leaf-hub-spec-sync scaled
    deployment.apps/leaf-hub-status-sync scaled
    ```

1.  Delete the managed clusters:

    ```
    delete from status.managed_clusters;
    ```

1.  Scale to one leaf hub controllers on `hub1`:

    ```
    $ kubectl scale deployment -n open-cluster-management --kubeconfig $HUB1_CONFIG leaf-hub-spec-sync leaf-hub-status-sync --replicas 1
    deployment.apps/leaf-hub-spec-sync scaled
    deployment.apps/leaf-hub-status-sync scaled
    ```

1.  Wait several seconds and then run the following query:

    ```
    select cluster_name, leaf_hub_name, payload -> 'metadata' -> 'labels' as labels from status.managed_clusters order by cluster_name;
     cluster_name | leaf_hub_name |                            labels
    --------------+---------------+--------------------------------------------------------------
     cluster0     | hub1          | {"name": "cluster0", "vendor": "Kind", "environment": "dev"}
     cluster1     | hub1          | {"name": "cluster1", "vendor": "Kind"}
     cluster2     | hub1          | {"name": "cluster2", "vendor": "Kind"}
     cluster3     | hub1          | {"name": "cluster3", "vendor": "Kind", "environment": "dev"}
     cluster4     | hub1          | {"name": "cluster4", "vendor": "Kind"}
    ```

## The demo script

1.  Show managed clusters in the UI of `hoh`, `hub1` and `hub2`.

1.  Show managed clusters in the database:

    ```
    select cluster_name, leaf_hub_name, payload -> 'metadata' -> 'labels' as labels from status.managed_clusters order by cluster_name;
     cluster_name | leaf_hub_name |                            labels
    --------------+---------------+--------------------------------------------------------------
     cluster0     | hub1          | {"name": "cluster0", "vendor": "Kind", "environment": "dev"}
     cluster1     | hub1          | {"name": "cluster1", "vendor": "Kind"}
     cluster2     | hub1          | {"name": "cluster2", "vendor": "Kind"}
     cluster3     | hub1          | {"name": "cluster3", "vendor": "Kind", "environment": "dev"}
     cluster4     | hub1          | {"name": "cluster4", "vendor": "Kind"}
    ```

1.  Create a policy, a placement rule and placement binding by `kubectl`:

    ```
    $ kubectl apply -f policy-psp.yaml --kubeconfig $TOP_HUB_CONFIG -n myproject
    policy.policy.open-cluster-management.io/policy-podsecuritypolicy created
    placementbinding.policy.open-cluster-management.io/binding-policy-podsecuritypolicy created
    placementrule.apps.open-cluster-management.io/placement-policy-podsecuritypolicy created
    ```

1.  Observe the policy in the UI of `hoh` and `hub1`. The policy does not appear in the UI of `hub2` since its leaf hub
controllers do not run.

1.  Show the policy in the `spec.policies` table:

    ```
    select created_at, updated_at, payload -> 'metadata' ->> 'name' as name, payload -> 'metadata' ->> 'namespace' as namespace, payload -> 'spec' ->> 'remediationAction' as action, deleted, jsonb_pretty(payload) as payload from spec.policies;
    ```

1.  Change compliance of one of the managed clusters of `hub1`. Observe changes in the compliance UI of `hoh` and
`hub1`.

1.  "Connect" `hub2` by scaling to 1 its controllers:

    ```
    $ kubectl scale deployment -n open-cluster-management --kubeconfig $HUB2_CONFIG leaf-hub-spec-sync leaf-hub-status-sync --replicas 1
    deployment.apps/leaf-hub-spec-sync scaled
    deployment.apps/leaf-hub-status-sync scaled
    ```

1.  Notice the policy appears in the UI of `hub2`.

1.  Note that all the clusters are populated in the table of managed clusters:

    ```
    select cluster_name, leaf_hub_name, payload -> 'metadata' -> 'labels' as labels from status.managed_clusters order by cluster_name;
     cluster_name | leaf_hub_name |                            labels
    --------------+---------------+--------------------------------------------------------------
     cluster0     | hub1          | {"name": "cluster0", "vendor": "Kind", "environment": "dev"}
     cluster1     | hub1          | {"name": "cluster1", "vendor": "Kind"}
     cluster2     | hub1          | {"name": "cluster2", "vendor": "Kind"}
     cluster3     | hub1          | {"name": "cluster3", "vendor": "Kind", "environment": "dev"}
     cluster4     | hub1          | {"name": "cluster4", "vendor": "Kind"}
     cluster5     | hub2          | {"name": "cluster5", "vendor": "Kind"}
     cluster6     | hub2          | {"name": "cluster6", "vendor": "Kind"}
     cluster7     | hub2          | {"name": "cluster7", "vendor": "Kind", "environment": "dev"}
     cluster8     | hub2          | {"name": "cluster8", "vendor": "Kind", "environment": "dev"}
     cluster9     | hub2          | {"name": "cluster9", "vendor": "Kind", "environment": "dev"}
    (10 rows)
    ```

1.  Change compliance of one of the managed clusters of `hub2`, and observe the compliance status changes in the UI of
`hoh` and `hub2`.

1.  Perform various queries on the compliance of the policies:

    1.  Select non-compliant clusters:

        ```
        select p.payload -> 'metadata' ->> 'name' as policyname, p.payload -> 'metadata' ->> 'namespace' as policynamespace, c.cluster_name, c.leaf_hub_name, c.compliance from spec.policies p INNER JOIN status.compliance c ON p.id = c.policy_id where c.compliance = 'non_compliant';
                policyname        | policynamespace | cluster_name | leaf_hub_name |  compliance
        --------------------------+-----------------+--------------+---------------+---------------
         policy-podsecuritypolicy | myproject       | cluster7     | hub2          | non_compliant
         policy-podsecuritypolicy | myproject       | cluster9     | hub2          | non_compliant
         policy-podsecuritypolicy | myproject       | cluster0     | hub1          | non_compliant
        ```

    1.  Select compliant clusters:

        ```
        select p.payload -> 'metadata' ->> 'name' as policyname, p.payload -> 'metadata' ->> 'namespace' as policynamespace, c.cluster_name, c.leaf_hub_name, c.compliance from spec.policies p INNER JOIN status.compliance c ON p.id = c.policy_id where c.compliance = 'compliant';
        ```

    1.  Count non-compliant clusters per leaf hub:

        ```
        select leaf_hub_name, count(cluster_name) as non_compliant_clusters_number from status.compliance where compliance = 'non_compliant' group by leaf_hub_name;
        leaf_hub_name | non_compliant_clusters_number
        ---------------+-------------------------------
        hub2          |                             2
        hub1          |                             1
        ```

1.  Change the remediation action in the UI of `hoh` to `enforce`. Observe propagation of changes and the status.

1.  Observe the change of the remediation action in `spec.policies`:

    ```
    select created_at, updated_at, payload -> 'metadata' ->> 'name' as name, payload -> 'metadata' ->> 'namespace' as namespace, payload -> 'spec' ->> 'remediationAction' as action, deleted, jsonb_pretty(payload) as payload from spec.policies;
    ```

1.  Observe the change in the compliant clusters in the database:

    ```
    select p.payload -> 'metadata' ->> 'name' as policyname, p.payload -> 'metadata' ->> 'namespace' as policynamespace, c.cluster_name, c.leaf_hub_name, c.compliance from spec.policies p INNER JOIN status.compliance c ON p.id = c.policy_id where c.compliance = 'non_compliant';
    ```

    ```
    select p.payload -> 'metadata' ->> 'name' as policyname, p.payload -> 'metadata' ->> 'namespace' as policynamespace, c.cluster_name, c.leaf_hub_name, c.compliance from spec.policies p INNER JOIN status.compliance c ON p.id = c.policy_id where c.compliance = 'compliant';
    ```

1.  Delete the policy in the UI of `hoh`. Observe propagation of the deletion to `hub1` and `hub2`.

1.  Observe the change of the `deleted` field in `spec.policies`:

    ```
    select created_at, updated_at, payload -> 'metadata' ->> 'name' as name, payload -> 'metadata' ->> 'namespace' as namespace, deleted from spec.policies;
            created_at         |         updated_at         |           name           | namespace | deleted
    ---------------------------+----------------------------+--------------------------+-----------+---------
     2021-07-19 09:18:03.85269 | 2021-07-19 10:03:33.869731 | policy-podsecuritypolicy | myproject | t
    ```
