# Deployment instructions for Hub of Hubs

## Prerequisites

1. ACM to serve as Hub of Hubs and ACM Hubs to connect to Hub of Hubs
1. The following command line tools installed:
    1. bash
    1. git
    1. helm 3.7+ or higher
    1. kubectl
    1. curl
    1. envsubst
    1. grep
    1. yq 4+

##  Set environment variables before deployment

1.  Set the `TOP_HUB_CONFIG` variable to hold the file path for kubernetes configuration of Hub of Hubs:
    ```
    export TOP_HUB_CONFIG=...
    ```

1.  Set the release tag variable for images:
    ```
    export TAG=v0.3.0
    ```
    
1.  Set the chosen transport. the options are either `kafka` or `sync-service`:
    ```
    export TRANSPORT_TYPE=kafka
    ``` 

----

# Hub of Hubs

### Deploying Hub-of-hubs components

```
KUBECONFIG=$TOP_HUB_CONFIG ./deploy_hub_of_hubs.sh
```

### Using Hub-of-Hubs Web Console

The Hub-of-Hubs Web console displayes managed clusters according to [Hub-of-Hubs RBAC](https://github.com/stolostron/hub-of-hubs-rbac). By default,
the `kube-admin` user is given `admin` role in Hub-of-Hubs RBAC. If you want to give other OpenShift users access to managed clusters, you need to
[configure RBAC role bindings for your users](https://github.com/stolostron/hub-of-hubs-rbac/blob/main/README.md#update-role-bindings-or-role-definitions).

### Undeploying Hub-of-hubs components

```
KUBECONFIG=$TOP_HUB_CONFIG ./undeploy_hub_of_hubs.sh
```

----

# Hubs to connect to Hub of Hubs

## Deploying a Hub-of-Hubs agent

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./deploy_hub_of_hubs_agent.sh
```

## Undeploying a Hub-of-Hubs agent

```
KUBECONFIG=$HUB1_CONFIG LH_ID=hub1 ./undeploy_hub_of_hubs_agent.sh
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
