# The Hub-of-Hubs demo script

1. The user observes managed clusters in the Clusters view.
1. The user defines a policy, a placement rule and a placement rule binding in their namespace, by ACM UI or kubectl.
1. The user observes policy compliance status in the UI of hub-of-hubs and the UI of leaf hubs.

## Prequisites

* Three clusters with ACM Hubs on them, `hoh` (the hub of hubs), `hub1` and `hub2`.
* Hub-of-Hubs components [deployed](https://github.com/stolostron/hub-of-hubs/blob/main/deploy/README.md) on `hoh`, Leaf Hub components [deployed](https://github.com/stolostron/hub-of-hubs/blob/main/deploy/README.md) `hub1` and `hub2`.
* Environment variables hold kubernetes configurations of the clusters,
`TOP_HUB_CONFIG`, `HUB1_CONFIG` and `HUB2_CONFIG`.


## The demo script

1.  Observe managed clusters in the UI of `hoh`, `hub1` and `hub2`. Note that the UI of `hoh` contains managed clusters of both `hub1` and `hub2`.

1.  Create a policy, a placement rule and placement binding by `kubectl`:

    ```
    $ kubectl apply -f policy-psp.yaml --kubeconfig $TOP_HUB_CONFIG
    policy.policy.open-cluster-management.io/policy-podsecuritypolicy created
    placementbinding.policy.open-cluster-management.io/binding-policy-podsecuritypolicy created
    placementrule.apps.open-cluster-management.io/placement-policy-podsecuritypolicy created
    ```

1.  Observe the policy in the UI of `hoh`, `hub1` and `hub2`.

1.  Change compliance of one of the managed clusters of `hub1`. Observe changes in the compliance UI of `hoh` and
`hub1`.

1.  Change the remediation action in the UI of `hoh` to `enforce`. Observe propagation of changes and the status.

1.  Delete the policy in the UI of `hoh`. Observe propagation of the deletion to `hub1` and `hub2`.
