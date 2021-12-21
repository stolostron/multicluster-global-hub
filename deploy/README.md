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

1.  Set the `TOP_HUB_CONFIG` variable to hold the file path for kubernetes configuration of Hub-of-Hubs:
    ```
    export TOP_HUB_CONFIG=...
    ```

1.  Set the release tag variable for images:
    ```
    export TAG=v0.2.0
    ```
    
1.  Set the chosen transport. the options are either `kafka` or `sync-service`:
    ```
    export TRANSPORT_TYPE=kafka
    ``` 
    
    1.  If `sync-service` is set as transport, set environment variable to hold the CSS host:
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
