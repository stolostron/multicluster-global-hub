# Multicluster Global Hub

This document focuses on the features of the multicluster global hub.

- [Multicluster Global Hub](#multicluster-global-hub)
  - [Overview](#overview)
  - [Use Cases](./global_hub_use_cases.md)
  - [Architecture](#architecture)
    - [Multicluster Global Hub Operator](#multicluster-global-hub-operator)
    - [Multicluster Global Hub Manager](#multicluster-global-hub-manager)
    - [Multicluster Global Hub Agent](#multicluster-global-hub-agent)
    - [Multicluster Global Hub Observability](#multicluster-global-hub-observability)
  - [Workings of Global Hub](./how_global_hub_works.md)
  - [Quick Start](#quick-start)
    - [Prerequisites](#prerequisites)
      - [Dependencies](#dependencies)
      - [Network configuration](#network-configuration)
    - [Installation](#installation)
      - [1. Install the multicluster global hub operator on a disconnected environment](#1-install-the-multicluster-global-hub-operator-on-a-disconnected-environment)
      - [2. Install the multicluster global hub operator from OpenShift console](#2-install-the-multicluster-global-hub-operator-from-openshift-console)
    - [Import a regional hub cluster in default mode (tech preview)](#import-a-regional-hub-cluster-in-default-mode-tech-preview)
    - [Access the grafana](#access-the-grafana)
    - [Grafana dashboards](#grafana-dashboards)
  - [Troubleshooting](./troubleshooting.md)
  - [Development preview features](./dev-preview.md)
  - [Known issues](#known-issues)

## Overview

The multicluster global hub is to resolve the problem of a single hub cluster in high scale environment. Due to the limitation of the kubernetes, the single hub cluster can not handle the large number of managed clusters. The multicluster global hub is designed to solve this problem by splitting the managed clusters into multiple regional hub clusters. The regional hub clusters are managed by the global hub cluster.

The multicluster global hub is a set of components that enable the management of multiple clusters from a single hub cluster. It is designed to be deployed on a hub cluster and provides the following features:

- Deploy the regional hub clusters
- List the managed clusters in all the regional hub clusters
- Manage the policies and applications in all the regional hub clusters

## Use Cases

For understanding the Use Cases solved by Global Hub proceed to [Use Cases](./global_hub_use_cases.md)

## Architecture

![ArchitectureDiagram](architecture/multicluster-global-hub-arch.png)

### Multicluster Global Hub Operator

Operator is for multicluster global hub. It is used to deploy all required components for multicluster management. The components include multicluster-global-hub-manager in the global hub cluster and multicluster-global-hub-agent in the regional hub clusters.

The Operator also leverages the manifestwork to deploy the Advanced Cluster Management for Kubernetes in the managed cluster. So the managed cluster is switched to a standard ACM Hub cluster (regional hub cluster).

### Multicluster Global Hub Manager

The manager is used to persist the data into the postgreSQL. The data is from Kafka transport. The manager is also used to post the data to Kafka transport so that it can be synced to the regional hub clusters.

### Multicluster Global Hub Agent

The agent is running in the regional hub clusters. It is responsible to sync-up the data between the global hub cluster and the regional hub clusters. For instance, sync-up the managed clusters' info from the regional hub clusters to the global hub cluster and sync-up the policy or application from the global hub cluster to the regional hub clusters.

### Multicluster Global Hub Observability

Grafana runs on the global hub cluster as the main service for Global Hub Observability. The Postgres data collected by the Global Hub Manager services as its default DataSource. By exposing the service via route(`multicluster-global-hub-grafana`), you can access the global hub grafana dashboards just like accessing the openshift console.

## Workings of Global Hub

To understand how Global Hub functions, proceed [here](how_global_hub_works.md).

## Quick Start

### Prerequisites
#### Dependencies
1. **Red Hat Advanced Cluster Management for Kubernetes (RHACM)** 2.7 or later needs to be installed

    [Learn more details about RHACM](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7)

2. **Crunchy Postgres for Kubernetes** 5.0 or later needs to be provided

    **Crunchy Postgres for Kubernetes** provide a declarative Postgres solution that automatically manages PostgreSQL clusters.
    
    [Learn more details about Crunchy Postgres for Kubernetes](https://access.crunchydata.com/documentation/postgres-operator/v5/)

    Global hub manager and grafana services need Postgres database to collect and display data. The data can be accessed by creating a storage secret `multicluster-global-hub-storage` in namespace `open-cluster-management`, this secret should contains the following two fields:

    - `database_uri`: Required, the URI user should have the permission to create the global hub database in the postgres.
    - `ca.crt`: Optional, if your database service has TLS enabled, you can provide the appropriate certificate depending on the SSL mode of the connection. If the SSL mode is `verify-ca` and `verify-full`, then the `ca.crt` certificate must be provided.

    > Note: There is a sample script available [here](https://github.com/stolostron/multicluster-global-hub/tree/main/operator/config/samples/storage)(Note:the client version of kubectl must be v1.21+) to install postgres in `hoh-postgres` namespace and create the secret `multicluster-global-hub-storage` in namespace `open-cluster-management` automatically.

3. **Strimzi** 0.33 or later needs to be provided

    **Strimzi** provides a way to run Kafka cluster on Kubernetes in various deployment configurations. 
    
    [Learn more details about Strimzi](https://strimzi.io/documentation/)

    Global hub agent need to sync cluster info and policy info to Kafka transport. And global hub manager persist the Kafka transport data to Postgre database.

    So, you need to create a secret `multicluster-global-hub-transport` in global hub cluster namespace `open-cluster-management` for the Kafka transport. The secret contains the following fields:

    - `bootstrap.servers`: Required, the Kafka bootstrap servers.
    - `ca.crt`: Optional, if you use the `KafkaUser` custom resource to configure authentication credentials, you can follow this [document](https://strimzi.io/docs/operators/latest/deploying.html#con-securing-client-authentication-str) to get the `ca.crt` certificate from the secret.
    - `client.crt`: Optional, you can follow this [document](https://strimzi.io/docs/operators/latest/deploying.html#con-securing-client-authentication-str) to get the `user.crt` certificate from the secret.
    - `client.key`: Optional, you can follow this [document](https://strimzi.io/docs/operators/latest/deploying.html#con-securing-client-authentication-str) to get the `user.key` from the secret.

    > Note: There is a sample script available [here](https://github.com/stolostron/multicluster-global-hub/tree/main/operator/config/samples/transport) to install kafka in `kafka` namespace and create the secret `multicluster-global-hub-transport` in namespace `open-cluster-management` automatically.

#### Sizing
1. [Sizing your RHACM cluster](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html/install/installing#sizing-your-cluster)

2. **Minimum requirements for Crunchy Postgres**

    | vCPU | Memory | Storage size | Namespace |
    | ---- | ------ | ------ | ------ |
    | 100m | 2G | 20Gi*3 | hoh-postgres
    | 10m | 500M | N/A | postgres-operator
    
3. **Minimum requirements for Strimzi**

    | vCPU | Memory | Storage size | Namespace |
    | ---- | ------ | ------ | ------ |
    | 100m | 8G | 20Gi*3 | kafka


#### Network configuration
As regional hub is also managedcluster of global hub in RHACM. So the network configuration in RHACM is necessary. Details see [RHACM Networking](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html/networking/networking)

1. Global hub networking requirements

| Direction | Protocol | Connection | Port (if specified) | Source address |	Destination address |
| ------ | ------ | ------ | ------ |------ | ------ |
|Inbound from user's browsers | HTTPS | User need to access the grafana dashboard | 443 | User's browsers | IP address of grafana route |
| Outbound to Kafka Cluster | HTTPS | Global hub manager need to get data from Kafka cluster | 443 | multicluster-global-hub-manager-xxx pod | Kafka route host |
| Outbound to Postgres database | HTTPS | Global hub manager need to persist data to Postgres database | 443 | multicluster-global-hub-manager-xxx pod | IP address of Postgres database |

2. Regional hub networking requirements

| Direction | Protocol | Connection | Port (if specified) | Source address |	Destination address |
| ------ | ------ | ------ | ------ | ------ | ------ |
| Outbound to Kafka Cluster | HTTPS | Global hub agent need to sync cluster info and policy info to Kafka cluster | 443 | multicluster-global-hub-agent pod | Kafka route host |

### Installation

#### 1. [Install the multicluster global hub operator on a disconnected environment](./disconnected_environment/README.md)

#### 2. Install the multicluster global hub operator from OpenShift console

1. Log in to the OpenShift console as a user with cluster-admin role.
2. Click the Operators -> OperatorHub icon in the left navigation panel.
3. Search for the `multicluster global hub operator`.
4. Click the `multicluster global hub operator` to start the installation.
5. Click the `Install` button to start the installation when you are ready.
6. Wait for the installation to complete. You can check the status in the `Installed Operators` page.
7. Click the `multicluster global hub operator` to go to the operator page.
8. Click the `multicluster global hub` tab to see the `multicluster global hub` instance.
9. Click the `Create multicluster global hub` button to create the `multicluster global hub` instance.
10. Fill in the required information and click the `Create` button to create the `multicluster global hub` instance.

> Note: the multicluster global hub is available for x86 platform only right now.
> Note: The policy and application are disabled in RHACM once the multicluster global hub is installed.

### Import a regional hub cluster in default mode (tech preview)

It requires to disable the cluster self management in the existing ACM hub cluster. Set `disableHubSelfManagement=true` in the `multiclusterhub` CR to disable importing of the hub cluster as a managed cluster automaticially.

After that, follow the [Import cluster](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html-single/clusters/index#importing-a-target-managed-cluster-to-the-hub-cluster) document to import the regional hub cluster.

Once the regional hub cluster is imported, you can check the global hub agent status to ensure that the agent is running in the regional hub cluster.

```bash
oc get managedclusteraddon multicluster-global-hub-controller -n ${REGIONAL_HUB_CLUSTER_NAME}
```

### Access the grafana

The grafana is exposed through Route, you can use the following command to get the login URL. The authentication method of this URL is same as the openshift console, so you don't have to worry about using another authentication.

```bash
oc get route multicluster-global-hub-grafana -n <the-namespace-of-multicluster-global-hub-instance>
```

### Grafana dashboards

Upon accessing the global hub Grafana, users can begin monitoring the policies that have been configured through the hub cluster environments being managed. From the global hub dashboard, users can identify the compliance status of their system's policies over a selected time range. The policy compliance status is updated daily, so users can expect the dashboard to display the status of the current day on the following day.

![Global Hub Policy Group Compliancy Overview](./images/global-hub-policy-group-compliancy-overview.gif)

To navigate the global hub dashboards, users can choose to observe and filter the policy data by grouping them either by `policy` or `cluster`. If the user prefers to examine the policy data by `policy` grouping, they should start from the default dashboard called `Global Hub - Policy Group Compliancy Overview`. This dashboard allows users to filter the policy data based on `standard`, `category`, and `control`. Upon selecting a specific point in time on the graph, users will be directed to the `Global Hub - Offending Policies` dashboard, which lists the non-compliant or unknown policies at that particular time. After selecting a target policy, users can view related events and see what has changed by accessing the `Global Hub - What's Changed / Policies` dashboard.

![Global Hub Cluster Group Compliancy Overview](./images/global-hub-cluster-group-compliancy-overview.gif)

Similarly, if users prefer to examine the policy data by `cluster` grouping, they should begin from the `Global Hub - Cluster Group Compliancy Overview` dashboard. The navigation flow is identical to the `policy` grouping flow, but users will select filters related to the cluster, such as managed cluster `labels` and `values`. Instead of viewing policy events for all clusters, upon reaching the `Global Hub - What's Changed / Clusters` dashboard, users will be able to view policy events specifically related to an individual cluster.

## Troubleshooting

For common Troubleshooting issues, proceed [here](troubleshooting.md)

## Known issues

1. If the database is empty, the grafana dashboards will show the error `db query syntax error for {dashboard_name} dashboard`. When you have some data in the database, the error will disappear. Remember the Top level dashboards gets populated only the day after the data starts flowing as explained in [Workings of Global Hub](how_global_hub_works.md)

2. We provide ability to drill down the `Offending Policies` dashboard when you click a datapoint from the `Policy Group Compliancy Overview` dashboard. But the drill down feature is not working for the first datapoint. You can click the second datapoint or after to see the drill down feature is working. The issue is applied to the `Cluster Group Compliancy Overview` dashboard as well.

3. If you detach the regional hub and then rejoin the regional hub, The data (policies/managed clusters) might not be updated in time from the rejoined regional hub. You can fix this problem by restarting the `multicluster-global-hub-manager` pod on global hub.

4. For cluster that are never created successfully(clusterclaim id.k8s.io does not exist in the managed cluster), then we will not count this managed cluster in global hub policy compliance database, but it shows in RHACM policy console.
