# Setup the Environment for Global Hub Scale Test

## Initialize the clusters for global hub

You can execute the following script to create the hub clusters and join them into the global hub. To join these clusters to it, You must set the `KUBECONFIG` environment variable to enable these hubs can connect to the global hub. Besides, you also need to provide several parameters:

> If you have installed the release version of ACM for the global hub, you need to [configure the image pull secret](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.12/html-single/clusters/index#import-gui-prereqs) for the KinD cluster.

```bash
./doc/simulation/setup/setup-cluster.sh 1:5 1:300 
```

- `$1` - <hub_start:hub_end> - Managed hubs, from `hub1` to `hub5`
- `$2` - <cluster_start:cluster_end> - Managed clusters on each hub, from `managedcluter-1` to `managedcluster-300`

That means create `5` managed hubs and each has `300` managed clusters.

## Create the policies on the managed hub clusters

Running the following script to create the policies on all the managed hubs.

```bash
./doc/simulation/setup/setup-policy.sh 6:10 1:50 
```

- `$1` - <hub_start:hub_end> - Managed hubs, from `hub1` to `hub5`
- `$2` - <policy_start:policy_end> - Policies on each hub, from `rootpoicy-1` to `rootpolicy-50`

That means the operation will run on the `5` managed hub concurrently. Each of them will create `50` root policies and propagate to the `300` managed clusters. So there will be `15000` replicas polices on the managed hub cluster. 

## The Scale for Global Hub Test

- `1500` Managed Cluster: `5` managed hubs each with `300` clusters
- `150` Root Policies and `75000` Replicas Policies: each hub with `50` root policies and `50 * 300` replicas policies
- `75000` Events: each replicas polices with `1` event


## Start the Global Hub Agent on the Managed Hubs

_Note: Before running the agent on each hub, you can run the [backend process](../inspector/README.md#count-the-records-of-database) to monitor the changes in data in the database._

Add the label `vendor: OpenShift` to the hub clusters to start the `multicluster-global-hub-agent` on each managed hub.
  
```bash
kubectl label mcl hub1 vendor=OpenShift --overwrite
kubectl label mcl hub2 vendor=OpenShift --overwrite
kubectl label mcl hub3 vendor=OpenShift --overwrite
kubectl label mcl hub4 vendor=OpenShift --overwrite
kubectl label mcl hub5 vendor=OpenShift --overwrite
```

## Rotate the Status of policy

You can run the following script to update the replicas policies status on each hub cluster.

```bash
# update the 1 ~ 50 root policy on all the clusters, and update the status to Compliant(default NonCompliant)
$ ./doc/simulation/setup/rotate-policy.sh 1:5 1:50 "Compliant"
# ./doc/simulation/setup/rotate-policy.sh 1:5 1:50 "NonCompliant"
```

- `$1` - <hub_start:hub_end> - Managed hubs, from `hub1` to `hub5`
- `$2` - <policy_start:policy_end> - Policies on each hub, from `rootpoicy-1` to `rootpolicy-50`
- `$2` - The target compliance status


## Note

When creating these KinD clusters on your machine, please ensure that the KinD cluster is not running due to the following resource limitations.

```
sudo sysctl fs.inotify.max_user_instances=8192 >> /etc/sysctl.conf
sudo sysctl fs.inotify.max_user_watches=524288 >> /etc/sysctl.conf
sudo sysctl -p
```