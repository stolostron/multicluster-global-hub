# The Hub-of-Hubs demo script

1. The user defines a policy, a placement rule and a placement rule binding in their namespace, by ACM UI or kubectl.
1. They observe policy compliance status in the UI.
1. They observe managed clusters in the database.

## Prequisites

* Three clusters with ACM Hubs on them, `hoh` (the hub of hubs), `hub1` and `hub2`.
* Environment variables hold kubernetes configurations of the clusters,
`TOP_HUB_CONFIG`, `HUB1_CONFIG` and `HUB2_CONFIG`.
* Leaf hub components installed on `hub1` and `hub2`, hub-of-hubs spec and status sync controllers run on `hoh` cluster.
* Tranport bridge and transport server-side run on `hoh` or elsewhere.
* The database can be accessed for querying/deleting

### Before the demo

1. Scale to 0 leaf hub controllers on `hub1` and `hub2`:

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

1. Delete the managed clusters:

```
delete from status.managed_clusters;
```

1. Scale to one leaf hub controllers on `hub1`:

```
$ kubectl scale deployment -n open-cluster-management --kubeconfig $HUB1_CONFIG leaf-hub-spec-sync leaf-hub-status-sync --replicas 1
deployment.apps/leaf-hub-spec-sync scaled
deployment.apps/leaf-hub-status-sync scaled
```
