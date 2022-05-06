# Setup instructions and demo

![Hub-of-Hubs setup](animation.gif)

## Prerequisites

See [the common prerequisites for Hub-of-Hubs demos](../README.md#prerequisites).

## Installing Hub-of-Hubs components manually

1.  [Deploy Hub-of-Hubs components](https://github.com/stolostron/hub-of-hubs/blob/main/deploy/README.md) on `hoh`.
1.  [Deploy Hub-of-Hubs-agent components](https://github.com/stolostron/hub-of-hubs/tree/main/deploy#deploying-a-hub-of-hubs-agent) on `hub1` and `hub2`.

## Installing Hub-of-Hubs components by a demo shell script

You can demonstrate the setup itself (it could take up to 30 mintues) by running the shell script below **in this directory**. 
The shell script uses [demo-magic](https://github.com/paxtonhare/demo-magic), you need to press Enter to run commands one by one. 
The redirection of the standard error stream to `/dev/null` is required to suppress throttling warnings that expose the API server URL. Exposing API server URL could be undesirable for security.

```
./run.sh 2> /dev/null
```

## Cleanup

```
./clean.sh
```

## Install KinD clusters with ocm as leaf hub clusters
To quickly test your hub-of-hubs, you can choose to bootstrap several OCM clusters locally. Then you can import the hub clusters as the managed clusters of hub of hubs cluster. 
By running the shell script `./ocm_setup.sh`, you can get the local OCM cluster which has enabled the application lifecycle and policy.
You can control the number and size of OCM clusters by specifying environment variables **HUB_CLUSTER_NUM** and **MANAGED_CLUSTER_NUM**.
The default leaf hub cluster names are hub1, hub2, hub3, etc. And the managed clusters of the leaf hub are hub1-cluster1, hub1-cluster2, hub2-cluster1, etc.

```
export HUB_CLUSTER_NUM=1
export MANAGED_CLUSTER_NUM=2
./ocm_setup.sh
```
You can see the progress of creating the cluster by running the above script directly. Also check the logs in ./config/setup.log.

## Cleanup the KinD clusters
```
./ocm_clean.sh
```