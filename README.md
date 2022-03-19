# Hub of Hubs

[![License](https://img.shields.io/github/license/stolostron/hub-of-hubs)](/LICENSE)

----

Hub of Hubs provides multi-cluster management at very high scale.

Hub of Hubs utilizes several standard [RHACM hub clusters](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/about/welcome-to-red-hat-advanced-cluster-management-for-kubernetes#hub-cluster)
to provide cluster-management capabilities, such as applying compliance policies to managed clusters and monitoring
compliance status, or managing application and cluster lifecycle, while addressing the scale and performance
requirements of edge use cases.

Our [simulations](simulation.md) showed that a Hub of Hubs is able to manage one million clusters with hundreds of
policies and/or applications applied to each cluster.

![ArchitectureDiagram](images/ScalableHOHArchitecture.png)

----

## Getting started

The [demos](demos).

## Deployment

See [deployment scripts](deploy).

## Repositories

### Ansible playbooks to setup PostgreSQL
* [hub-of-hubs-postgresql](https://github.com/stolostron/hub-of-hubs-postgresql)

### Hub-of-Hubs components

* [hub-of-hubs-spec-sync](https://github.com/stolostron/hub-of-hubs-spec-sync)
* [hub-of-hubs-status-sync](https://github.com/stolostron/hub-of-hubs-status-sync)
* [hub-of-hubs-spec-transport-bridge](https://github.com/stolostron/hub-of-hubs-spec-transport-bridge)
* [hub-of-hubs-status-transport-bridge](https://github.com/stolostron/hub-of-hubs-status-transport-bridge)

#### RBAC

* [hub-of-hubs-rbac](https://github.com/stolostron/hub-of-hubs-rbac)

#### Non-Kubernetes REST API

* [hub-of-hubs-nonk8s-api](https://github.com/stolostron/hub-of-hubs-nonk8s-api)

#### GitOps

* [hub-of-hubs-gitops](https://github.com/stolostron/hub-of-hubs-gitops)

#### UI

* [hub-of-hubs-console](https://github.com/stolostron/hub-of-hubs-console)
* [hub-of-hubs-console-chart](https://github.com/stolostron/hub-of-hubs-console-chart)
* [hub-of-hubs-ui-components](https://github.com/stolostron/hub-of-hubs-ui-components)
* [hub-of-hubs-grc-ui](https://github.com/stolostron/hub-of-hubs-grc-ui)

### Transport options

* [hub-of-hubs-sync-service](https://github.com/stolostron/hub-of-hubs-sync-service)
* [hub-of-hubs-kafka-transport](https://github.com/stolostron/hub-of-hubs-kafka-transport)

### Message Compression

* [hub-of-hubs-message-compression](https://github.com/stolostron/hub-of-hubs-message-compression)

### Hub-of-Hubs-agent components

* [leaf-hub-spec-sync](https://github.com/stolostron/leaf-hub-spec-sync)
* [leaf-hub-status-sync](https://github.com/stolostron/leaf-hub-status-sync)

### Data types definitions

* [hub-of-hubs-crds](https://github.com/stolostron/hub-of-hubs-crds)
* [hub-of-hubs-data-types](https://github.com/stolostron/hub-of-hubs-data-types)

### Custom repositories

* Hub of Hubs
  * https://github.com/vadimeisenbergibm/governance-policy-propagator/tree/no_status_update to prevent updating policy status by governance policy propagator

## Development

See [the development page](./development.md).

### Simulation at high scale

See [simulation at high scale](./simulation.md).

### Contributing

Go to the [Contributing guide](CONTRIBUTING.md) to learn how to get involved.
