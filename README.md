# Hub-of-Hubs

[![License](https://img.shields.io/github/license/open-cluster-management/hub-of-hubs)](/LICENSE)

The main repository for the Hub-of-Hubs.

![ArchitectureDiagram](images/ScalableHOHArchitecture.png)

## Demo

The [demo script](demo.md).

## Repositories

### Ansible playbooks to setup PostgreSQL
* [hub-of-hubs-postgresql](https://github.com/open-cluster-management/hub-of-hubs-postgresql)

### On-the-hub-of-hubs components

* [hub-of-hubs-spec-sync](https://github.com/open-cluster-management/hub-of-hubs-spec-sync)
* [hub-of-hubs-status-sync](https://github.com/open-cluster-management/hub-of-hubs-status-sync)
* [hub-of-hubs-spec-transport-bridge](https://github.com/open-cluster-management/hub-of-hubs-spec-transport-bridge)
* [hub-of-hubs-status-transport-bridge](https://github.com/open-cluster-management/hub-of-hubs-status-transport-bridge)

### Transport options

* [hub-of-hubs-sync-service](https://github.com/open-cluster-management/hub-of-hubs-sync-service)
* [hub-of-hubs-kafka-transport](https://github.com/open-cluster-management/hub-of-hubs-kafka-transport)

### On-the-leaf-hub components

* [leaf-hub-spec-sync](https://github.com/open-cluster-management/leaf-hub-spec-sync)
* [leaf-hub-status-sync](https://github.com/open-cluster-management/leaf-hub-status-sync)

### Data types definitions

* [hub-of-hubs-crds](https://github.com/open-cluster-management/hub-of-hubs-crds)
* [hub-of-hubs-data-types](https://github.com/open-cluster-management/hub-of-hubs-data-types)

## Deployment

See [deployment scripts](deploy).

## Development

### Go version
 
1.16

### Conventions

#### Versioning

We use [the go modules semantic versioning convention](https://blog.golang.org/publishing-go-modules#TOC_3.), the git tags of the form `vMAJOR.MINOR.PATCH`. The major version for this POC is `0`. The depending modules should specify the versions of the hub-of-hubs repositories explicitly, and not use pseudo-verisons (like `v0.0.0-20200610161514-939cead3902c`).

The hub-of-hubs modules should use common versions of the dependencies. See [the list of the dependencies, and the versions to use in the POC](versions.md).

#### Context usage

`Context.Background()` should be defined in the beginning of each "main" method, such as `Reconcile`, or of a method to be called as a go routine,  and then passed to all the called methods. The functions that handle timers/timeouts/cancel events must handle cancelling the context.

References:
* http://p.agnihotry.com/post/understanding_the_context_package_in_golang/
* https://blog.golang.org/context

#### Errors vs logging

All the errors should be wrapped before returning, using `fmt.Errorf("Some description: %w", err)`. The errors should only be logged once in the "main" method, such as `Reconcile` or methods that do not return errors.

References:
* https://www.orsolabs.com/post/go-errors-and-logs/

#### Logging

We use controller-runtime logging and its [Logging guidelines](https://github.com/kubernetes-sigs/controller-runtime/blob/master/TMP-LOGGING.md).

#### Concurrency

We should use go routines where possible. Note that the database concurrency is limited by the maximal number of database connections (about 100). The multiple connections to a database are handled by pgx connection pool.

#### Using controller-runtime Manager

While using [controller-runtime Manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager) for controllers seems to be an obvious choice, 
using [controller-runtime's Manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager) for other components (for example for DB synchronizers) 
is not. We try to use [controller-runtime's Manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager) as much as possible, even for running components that are not controllers.

[controller-runtime's Manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager) provides the following functionality in addition to managing controllers:

   * Kubernetes client with caching
   * start/stop [Runnables](https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.0/pkg/manager#Runnable).
   * signal handling
   * leader election
   * metrics
   * health/ready checks
   * client-side event processing

See [an example of using controller-runtime Manager features](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/cronjob-tutorial/testdata/project/main.go).

#### Running as singletons

All the components are designed to run as singletons (a single active replica), since there is no load balancing between components. We use leader election of [controller-runtime's Manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager) to implement the singleton pattern.

#### Events

Currently we do not produce events.

#### Metrics

Currently we do not publish any metrics.

#### Health/ready checks

Currently we do not use health/ready checks.

### Build
 
* [Makefile](https://github.com/open-cluster-management/hub-of-hubs-spec-sync/blob/main/Makefile) 
* [Dockerfile](https://github.com/open-cluster-management/hub-of-hubs-spec-sync/blob/main/build/Dockerfile) 
* [build scripts](https://github.com/open-cluster-management/hub-of-hubs-spec-sync/tree/main/build/scripts)

### Formatting

* [GCI](https://github.com/daixiang0/gci) for ordering imports.
* [gofumpt](https://github.com/mvdan/gofumpt) for formatting (a stricter tool than `go fmt`).
* `go fmt`

### Linting

* `go vet`
* [golangci-lint](https://github.com/golangci/golangci-lint), the settings file is [.golangci.yaml](https://github.com/open-cluster-management/hub-of-hubs-spec-sync/blob/main/.golangci.yaml). 
* [golint](https://github.com/golang/lint)

❗Remember to copy [.golangci.yaml](https://github.com/open-cluster-management/hub-of-hubs-spec-sync/blob/main/.golangci.yaml) into your repo directory before linting.

ℹ️ If you want to specify something as false-positive, use the [//nolint](https://golangci-lint.run/usage/false-positives/) comment.

ℹ️ If you see stale errors from [golangci-lint](https://github.com/golangci/golangci-lint), run `golangci-lint cache clean`.

### Tests

We did not implement any unit/e2e tests for this POC.

### Simulation at high scale

In order to simulate at high scale a leaf-hub-status-sync component must be replaced with leaf-hub-simulator:
[leaf-hub-simulator](https://github.com/open-cluster-management/leaf-hub-status-sync/tree/leaf-hub-simulator)

Please follow the instructions at [leaf-hub-simulator](https://github.com/open-cluster-management/leaf-hub-status-sync/tree/leaf-hub-simulator) to deploy 'leaf-hub-status-sync' simulator.<br />
Pay attention on NUMBER_OF_SIMULATED_LEAF_HUBS environment variable - it defines number of **additional** (simulated) LHs. If the environment vairable is not provided or equal to 0 the simulator will behave as the original 'leaf-hub-statis-sync'.

The simulation uses following tools:
* [CLC simulator](https://github.com/hanqiuzh/acm-clc-scale) - creates, deletes or keeps alive mock managed clusters (ManagedCluster CR).
* [GRC simulator](https://github.com/open-cluster-management/grc-simulator) - patches policy (Policy CR) compliances.

**Recommendation** install the above tools on separate VM and run them with "nohup" command (nohup <cmd> &) to ensure tols are running even when terminal to the VM is diconnected.

Before starting the simulation ensure the environment is "clean":
* stop sync service at LH - kubectl scale deployment sync-service-ess -n sync-service --replicas 0
* stop hoh-status-transport-bridge at HoH - kubectl scale deployment hub-of-hubs-status-transport-bridge -n open-cluster-management --replicas 0
* stop leaf-hub-status-sync at LH - kubectl scale deployment leaf-hub-status-sync -n open-cluster-management --replicas 0
* clean database tables - delete from status.managed_clusters, delete from status.compliance
* clean sync service storage - you can use [edge-sync-service-client](https://github.com/open-horizon/edge-sync-service-client)
* start sync service at LH - kubectl scale deployment sync-service-ess -n sync-service --replicas 1
* start hoh-status-transport-bridge at HoH - kubectl scale deployment hub-of-hubs-status-transport-bridge -n open-cluster-management --replicas 1
* start leaf-hub-status-sync at LH - kubectl scale deployment leaf-hub-status-sync -n open-cluster-management --replicas 1
 
Run CLC simulator whether in "create mock-cluster" mode - it will create mock managed clusters and will keep them alive in case they weren't created before or in "keep-alive" mode to back mock managed clusters back to 'Ready' status.<br />
Once all managed clusters are ready you can run GRC simulator to patch policy compliance status periodically.






