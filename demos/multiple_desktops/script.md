# The Hub-of-Hubs multiple-desktop basic-demo script

1.  Login into the Web console of `hub1`. As the `hub1` user, observe managed clusters `cluster0` to `cluster4` in the
    Cluster view.

1.  In the terminal of the `hub1` user, run:

    ```
    kubectl get managedcluster
    ```

    You should see clusters `cluster0` to `cluster4` returned.

    ![Screenshot of the desktop of the hub1 user, Cluster view](images/hub1.png)

1.  Perform the previous two steps as the user of `hub2`.

    ![Screenshot of the desktop of the hub2 user, Cluster view](images/hub2.png)

1.  Login into the Web console of Hub of Hubs as the `kubeadmin` user. The Hub-of-Hubs Web console has the same URL as the original [ACM Web console URL](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/web_console/web-console#accessing-your-console).

    If you cannot login as `kubeadmin`, [add an alternative user as the admin to Hub-of-Hubs RBAC](https://github.com/stolostron/hub-of-hubs-rbac#update-role-bindings-or-role-definitions).

    Note that as a user of Hub of Hubs, you can observe the managed clusters of both `hub1` and `hub2`.

1.  Note that the managed clusters on `hoh` are not represented by Kubernetes Custom Resources (not stored in etcd),
    and cannot be queried by `kubectl`:

    ```
    $ kubectl get managedcluster
    No resources found
    ```

    ![Screenshot of the desktop of the Hub-of-Hubs user, Cluster view](images/hoh.png)

1.  Browse the Web console of Hub of Hubs. Note that the Web console of Hub of Hubs has a distinct color from the
    Web console of ACM Hubs. Also note that it has `Hub-of-Hubs,Experimental` in its title.

    Currently the Web console of Hub of Hubs has only three views, namely `Welcome`, `Clusters` and
    `Governance`. Also note that the Cluster view has neither tabs nor buttons to create or import a cluster.
    The cluster table does not have actions to detach a cluster or to edit cluster labels.

    ![Screenshot of the desktop of the Hub-of-Hubs user, Welcome view](images/hoh_welcome.png)

1.  As a user of `hub1`, add the `env=production` label to `cluster0` and `cluster3`, either by `kubectl`
    or
    [in the Web console](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/clusters/managing-your-clusters#managing-cluster-labels)
    of `hub1`.

    ```
    kubectl label managedcluster cluster0 env=production
    kubectl label managedcluster cluster3 env=production
    ```

    ![Screenshot of the desktop of the hub1 user, Cluster view with labels](images/hub1_labels.png)

1.  As a user of `hub2`, add the `env=production` label to `cluster7`, `cluster8` and `cluster9`.

    ```
    kubectl label managedcluster cluster7 env=production
    kubectl label managedcluster cluster8 env=production
    kubectl label managedcluster cluster9 env=production
    ```

    ![Screenshot of the desktop of the hub2 user, Cluster view with labels](images/hub2_labels.png)

1.  Note that the new labels appear in the Cluster View of Hub of Hubs.

    ![Screenshot of the desktop of the Hub-of-Hubs user, Cluster view with labels](images/hoh_labels.png)

1.  In the following steps you will apply a policy to managed clusters as a Hub-of-Hubs user. Verify in the
    Governance view of Hub-of-Hubs, `hub1` and `hub2` that currently no policies are defined.

    ![Screenshot of the desktop of the hub1 user, Governance view with no policies](images/hub1_no_policies.png)

    ![Screenshot of the desktop of the hub2 user, Governance view with no policies](images/hub2_no_policies.png)

    ![Screenshot of the desktop of the Hub-of-Hubs user, Governance view with no policies](images/hoh_no_policies.png)

1.  Create a policy, a placement rule and a placement binding in `hoh` cluster by `kubectl`. The policy used in the instructions below is an [ACM pod security policy](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/governance/governance#pod-security-policy). The placement rule selects clusters with the `env=production` label.

    ```
    $ kubectl apply -f https://raw.githubusercontent.com/stolostron/hub-of-hubs/main/demos/policy-psp.yaml --kubeconfig $TOP_HUB_CONFIG
    policy.policy.open-cluster-management.io/policy-podsecuritypolicy created
    placementbinding.policy.open-cluster-management.io/binding-policy-podsecuritypolicy created
    placementrule.apps.open-cluster-management.io/placement-policy-podsecuritypolicy created
    ```

1.  Observe the policy in the Web console of `hoh`, `hub1` and `hub2`.

1.  Click on the _Cluster violantions_ link in the Governance view of `hoh`, `hub1` and `hub2`.
    Note that all the managed clusters labeled with `env=production` from both `hub1` and `hub2` appear in the Cluster violiations view of `hoh`.

1.  Change compliance of one of the managed clusters of `hub1`. To make the managed cluster compliant, run:

    ```
    $ kubectl apply -f https://raw.githubusercontent.com/stolostron/hub-of-hubs/main/demos/psp.yaml --kubeconfig <a managed cluster config>
    ```

1.  Observe changes of the compliance status in the Web console of `hoh` and `hub1`.

1.  Change the remediation action in the Web console of `hoh` to `enforce`. Observe propagation of the changes and the status.

1.  Delete the policy in the Web console of `hoh`. Observe propagation of the deletion to `hub1` and `hub2`.
