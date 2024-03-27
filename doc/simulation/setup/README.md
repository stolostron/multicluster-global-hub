# Setup the Environment for Global Hub Scale Test

## Initialize the clusters for global hub

You can execute the following script to create the hub clusters and join them into the global hub. To join these clusters to it, You must set the `KUBECONFIG` environment variable to enable these hubs can connect to the global hub. Besides, you also need to provide several parameters:

```bash
./doc/simulation/setup/setup-cluster.sh 2 2000 
```

- `$1` - How many managed hub clusters will be created
- `$2` - How many managed cluster will be created on per managed hub
- `$3` - Which managed cluster to start on per managed hub, default value is `1`

That means create `5` managed hubs and each has `300` managed clusters. You can also run `./doc/simulation/managed-clusters/cleanup-cluster.sh 300` on each hub cluster to cleanup the generated managed clusters.

## Create the policies on the managed hub clusters

Running the following script to create the policies on all the managed hubs.

```bash
./doc/simulation/setup/setup-policy.sh 10 50 300
```

- `$1` - How many managed hub clusters to mock the polices
- `$2` - How many root policy will be created per managed hub cluster
- `$3` - How many managed cluster the root policy will be propagated to on each hub cluster

That means the operation will run on the `10` managed hub concurrently. Each of them will create `50` root policies and propagate to the `300` managed clusters. So there will be `15000` replicas polices on the managed hub cluster. Likewise, you can execute `./doc/simulation/local-policies/cleanup-policy.sh 50 300` on each managed hub to delete the created polices.

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

## Rotate the Status of Polcies

You can run the following script to update the replicas policies status on each hub cluster.

```bash
# update the 50 root policy on the 300 cluster, and update the status to Compliant(default NonCompliant)
$ ./doc/simulation/setup/rotate-policy.sh 50 300 "Compliant"
# $ ./doc/simulation/setup/rotate-policy.sh 50 300 "NonCompliant"
```
- `$1` - How many root policy status will route on per managed hub cluster
- `$2` - How many managed clusters will this `$1` poclies will rotate
- `$3` - The target compliance status
- `$4` - Optional: Specify how many processes can be executed concurrently