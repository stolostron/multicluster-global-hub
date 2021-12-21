# Deployment instructions for Hub-of-Hubs

## Prerequisites

1. Hub-of-Hubs ACM and Leaf Hub ACMs
1. The following command line tools installed:
    1. bash
    1. git
    1. helm
    1. kubectl
    1. curl
    1. envsubst
    1. sed
    1. grep

##  Set environment variables before deployment

1.  Set the `KUBECONFIG` variable to hold the kubernetes configuration of Hub-of-Hubs.

1.  Set the release tag variable for images:
    ```
    export TAG=v0.2.0
    ```
    
1.  Set the chosen transport. the options are either `kafka` or `sync-service`:
    ```
    export TRANSPORT_TYPE=kafka
    ``` 
    
    1.  in case `sync-service` is set as transport, Set environment variable to hold the CSS host:
        ```
        export CSS_SYNC_SERVICE_HOST=...
        ```
----

# Hub-of-Hubs

### Deploying Hub-of-hubs

```
KUBECONFIG=$TOP_HUB_CONFIG ./deploy_hub_of_hubs.sh
```

### Using Hub-of-Hubs UI

In order to use the Hub-of-Hubs UI, you need to
[configure RBAC role bindings for your users](https://github.com/open-cluster-management/hub-of-hubs-rbac/blob/main/README.md#update-role-bindings-or-role-definitions).

### Undeploying Hub-of-hubs

This script will remove kafka as well.
```
KUBECONFIG=$TOP_HUB_CONFIG ./undeploy_hub_of_hubs.sh
```

----

# Leaf-Hub

### Deploying Leaf-Hub transport

#### Setting Kafka as transport

if kafka is selected as transport, one needs to set the bootstrap servers and certificate environment variables.  
when Kafka is deployed in Hub-of-Hubs cluster using the automated script, it's possible to use `kubectl` to 
get the appropriate variables:

1.  Set the `KAFKA_BOOTSTRAP_SERVERS` environment variable:
    ```
    export KAFKA_BOOTSTRAP_SERVERS=$(KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].bootstrapServers})
    ``` 
    
1.  Get certificate content from kafka cluster:
    ```
    KUBECONFIG=$TOP_HUB_CONFIG kubectl -n kafka get Kafka kafka-brokers-cluster -o jsonpath={.status.listeners[1].certificates}
    ``` 
    
1.  Set the `KAFKA_SSL_CA` environment variable to hold the certificate content:
    ```
    export KAFKA_SSL_CA=...
    ```

#### Setting Sync-Service as transport  

Deploying Edge Sync Service (ESS) on a Leaf Hub is a one time deployment, no need to deploy it on version change of 
leaf hub components.

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./deploy_leaf_hub_sync_service.sh
```

## Undeploying Edge Sync Service (ESS) from a Leaf Hub

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./undeploy_leaf_hub_sync_service.sh
```

## Deploying a Leaf Hub

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./deploy_leaf_hub.sh
```

## Undeploying a Leaf Hub

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./undeploy_leaf_hub.sh
```

----

## Linting

**Prerequisite**: install the `shellcheck` tool (a Linter for shell):

```
brew install shellcheck
```

Run
```
shellcheck *.sh
```
