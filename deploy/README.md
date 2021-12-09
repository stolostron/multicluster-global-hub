# Deployment instructions for Hub-of-Hubs PoC

## Prerequisites

1. Hub-of-Hubs ACM and Leaf Hub ACMs
1. Hub-of-Hubs database
1. Cloud (the Server side) Sync service running
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

1.  Define the `KUBECONFIG` variable to hold the kubernetes configuration of Hub-of-Hubs

1.  Define the release tag variable for images:

    ```
    export TAG=v0.1.0
    ```

1.  Define the `DATABASE_URL_HOH` and `DATABASE_URL_TRANSPORT` variables for the HoH and Transport users

    Set the `DATABASE_URL` according to the PostgreSQL URL format: `postgres://YourUserName:YourURLEscapedPassword@YourHostname:5432/YourDatabaseName?sslmode=verify-full&pool_max_conns=50`.

    :exclamation: Remember to URL-escape the password, you can do it in bash:

    ```
    python -c "import sys, urllib as ul; print ul.quote_plus(sys.argv[1])" 'YourPassword'
    ```

1.  Define `SYNC_SERVICE_HOST` environment variable to hold the CSS host.

1.  Define `CSS_SYNC_SERVICE_PORT` environment variable to hold the CSS port, if not defined, default port `9689` is used.

## Deploying Hub-of-hubs

```
KUBECONFIG=$TOP_HUB_CONFIG ./deploy_hub_of_hubs.sh
```

## Using Hub-of-Hubs UI

In order to use the Hub-of-Hubs UI, you need to
[configure RBAC role bindings for your users](https://github.com/open-cluster-management/hub-of-hubs-rbac/blob/main/README.md#update-role-bindings-or-role-definitions).

## Undeploying Hub-of-hubs

```
KUBECONFIG=$TOP_HUB_CONFIG ./undeploy_hub_of_hubs.sh
```

## Deploying Edge Sync Service (ESS) on a Leaf Hub

ESS is a one time deployment, no need to deploy it on version change of leaf hub components

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

## Linting

**Prerequisite**: install the `shellcheck` tool (a Linter for shell):

```
brew install shellcheck
```

Run
```
shellcheck *.sh
```
