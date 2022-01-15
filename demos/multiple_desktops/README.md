# Multiple-Desktops Hub-of-Hubs demo

In this demo the presenter uses three different desktops to show the point of view of three different users:
of the Hub-of-Hubs user, and of the users of two Hubs connected to the Hubs of Hubs.
The presenter opens a browser window with ACM Web console and a terminal to issue CLI commands, in each of the desktops.
The `KUBECONFIG` of each terminal should point to the respective OpenShift Cluster.
The presenter might set different backgrounds to the desktops and the terminals, and put a note designating the user of
the desktop.
During the demo, the presenter should switch between the desktop and describe the user he/she is currently representing.

![Hub-of-Hubs Multiple Desktop, Cluster view](images/animation.gif)

# Description

1. The presenter shows all the managed clusters of the connected hubs in the Clusters view of the Hub-of-Hubs Web console.
1. The presenter defines a policy, a placement rule and a placement rule binding in their namespace, by Hub-of-Hubs Web console or
by CLI (`kubectl`, `oc`) of the Hub-of-Hubs OpenShift cluster.
1. The presenter shows policy compliance status in the Web console of Hub of Hubs and in the Web console of the hubs connected to the Hub of Hubs.

## Prerequisites

See [the common prerequisites for Hub-of-Hubs demos](../README.md#prerequisites).

## Setup

See [the common setup instructions for Hub-of-Hubs demos](../README.md#setup).

## The script

[The script](script.md) to be used as a reference for the presenter or as a self-learning tutorial.
The script assumes that `hub1` manages clusters named `cluster0`, `cluster1`,
`cluster2`, `cluster3` and `cluster4`.
`hub2` manages clusters named `cluster5`, `cluster6`, `cluster7`, `cluster8` and `cluster9`.

To run the commands of the demo as shell scripts, run the `run.sh` shell scripts
**in the directories that correspond to the users(`hoh`, `hub1` or `hub2`)** .
The shell scripts use [demo-magic](https://github.com/paxtonhare/demo-magic),
you need to press Enter to run commands one by one.
The redirection of the standard error stream to `/dev/null` is required to suppress
throttling warnings that expose the API server URL.
Exposing API server URL could be undesirable for security.

```
./run.sh 2> /dev/null
```

## Cleanup

```
../single_desktop/clean.sh
```
