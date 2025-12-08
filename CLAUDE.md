
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Multicluster Global Hub is a Kubernetes operator-based system for managing ACM/OCM at very high scale. The repository contains three main components:

- **operator**: Deploys and manages the Global Hub infrastructure
- **manager**: Runs on the global hub cluster to sync data to/from PostgreSQL and Kafka
- **agent**: Runs on managed hub clusters to sync data between global hub and managed hubs

Shared code lives in `pkg/`.

## Architecture

### Component Responsibilities

**Operator** (`operator/`):

- Deploys manager (global hub cluster) and agent (managed hub clusters)
- Manages CRD lifecycle and custom resources
- API definitions in `operator/api/`

**Manager** (`manager/`):

- `pkg/spec/`: Syncs resources FROM database TO managed hubs via Kafka
  - `specdb/`: Database operations for spec resources
  - `controllers/`: Watches and persists resources to database
  - `syncers/`: Retrieves from database and sends via transport
- `pkg/status/`: Syncs resource status FROM managed hubs TO database
  - `conflator/`: Merges bundles from transport before database insertion
  - `dispatcher/`: Routes bundles between transport, conflator, and database
  - `handlers/`: Persists transferred bundles to database
- `pkg/controllers/`: Common controllers (migration, backup)
- `pkg/processes/`: Periodic jobs (policy compliance cronjob, managed hub management)
- `pkg/restapis/`: REST APIs for managed clusters, policies, subscriptions
- `pkg/webhook/`: Webhooks for global resources

**Agent** (`agent/`):

- `pkg/spec/`: Applies resources FROM global hub TO managed hub cluster
  - `rbac/`: Role-based access control
  - `syncers/`: Syncs resources and signals from manager
  - `workers/`: Backend goroutines executing spec syncer tasks
- `pkg/status/`: Reports resource status FROM managed hub TO manager via Kafka/Inventory API
  - `filter/`: Deduplicates events
  - `generic/`: Templates for status syncers
    - `controller/`: Specifies resource types to sync
    - `handler/`: Updates bundles for watched resources
    - `emitter/`: Sends bundles via transport (CloudEvents)
    - `multi-event syncer`: Template for multiple events per object (policy syncer)
    - `multi-object syncer`: Template for one event per multiple objects (managedhub info syncer)
  - `syncers/`: Specific resource syncers using generic templates
- `pkg/controllers/inventory/`: Controllers reporting via Inventory API

**Shared** (`pkg/`):

- `transport/`: Kafka integration (Sarama and Confluent)
- `database/`: PostgreSQL operations (GORM and pgx)
- `bundle/`: Data bundling and compression
- `constants/`, `enum/`, `utils/`: Common utilities

## Build and Development Commands

### Building Images

```bash

# Build and push all component images
make vendor
make build-operator-image push-operator-image IMG=<registry>/multicluster-global-hub-operator:<tag>
make build-manager-image push-manager-image IMG=<registry>/multicluster-global-hub-manager:<tag>
make build-agent-image push-agent-image IMG=<registry>/multicluster-global-hub-agent:<tag>

# Individual component builds
cd operator && make docker-build docker-push IMG=<registry>/multicluster-global-hub-operator:<tag>
cd manager && make
cd agent && make
```

### Deploying

```bash

# Deploy operator to cluster
make deploy-operator  # or: cd operator && make deploy IMG=<registry>/...:tag

# Install Global Hub instance
kubectl apply -k operator/config/samples/

# Undeploy
make undeploy-operator  # or: cd operator && make undeploy
```

### Code Quality

```bash

# Format code (standard Go formatting)
make fmt

# Strict formatting (gci + gofumpt)
make strict-fmt

# Update dependencies
make tidy
make vendor
```

### Testing

**Unit Tests:**

```bash

# Run all unit tests (requires setup-envtest)
make unit-tests

# Run component-specific unit tests
make unit-tests-operator
make unit-tests-manager
make unit-tests-agent
make unit-tests-pkg
```

**Integration Tests:**

```bash

make integration-test                # All integration tests
make integration-test/operator
make integration-test/manager
make integration-test/agent
```

**E2E Tests:**

```bash

# Setup E2E environment (creates KinD clusters)
make e2e-setup

# Run specific E2E test suites
make e2e-test-cluster
make e2e-test-local-agent
make e2e-test-localpolicy
make e2e-test-grafana

# Run all E2E tests
make e2e-test-all

# Cleanup E2E environment
make e2e-cleanup

# E2E test with verbose output
make e2e-test-localpolicy VERBOSE=9
```

**Running Single Tests:**

To run a single test file or function:

```bash
# Unit test - single package
cd operator && KUBEBUILDER_ASSETS="$(setup-envtest use --use-env -p path)" go test -v ./pkg/path/to/package -run TestFunctionName

# Integration test - single test
KUBEBUILDER_ASSETS="$(setup-envtest use --use-env -p path)" go test -v ./test/integration/operator/... -run TestSpecificFunction
```

### Operator-Specific Commands

When modifying operator API definitions:


```bash
cd operator
make generate    # Generate code (DeepCopy, etc.)
make manifests   # Generate CRDs, RBAC, etc.
make bundle      # Generate operator bundle
```

### Logs

Fetch logs from E2E test environment:


```bash
make e2e-log/operator
make e2e-log/manager
make e2e-log/grafana
make e2e-log/agent
```

## Code Formatting Rules

The `make fmt` target enforces import dependency rules:

- `pkg/` must NOT import from `agent/`, `operator/`, or `manager/`
- `operator/` must NOT import from `agent/` or `manager/`
- `agent/` must NOT import from `manager/` or `operator/` (except `operator/api`)
- `manager/` must NOT import from `agent/` or `operator/` (except `operator/api`)

This maintains clean separation between components. Only shared code should live in `pkg/`.

## Testing Guidelines

- Integration tests use envtest and require KUBEBUILDER_ASSETS

- E2E tests create real KinD clusters (configured via MH_NUM, MC_NUM environment variables)
- Test scripts are in `test/script/`
- E2E test configuration is stored in a temporary `CONFIG_DIR`
- When debugging test failures, run tests individually with verbose flags (`-v` for go test, `VERBOSE=9` for E2E scripts)

## Key Dependencies

- **Kubernetes**: v0.33.x (client-go, api, apimachinery)

- **Controller Runtime**: v0.21.x
- **Kafka**: IBM Sarama v1.45.x, Confluent Kafka v2.12.x
- **Database**: GORM v1.30.x, pgx v5.7.x
- **CloudEvents**: v2.16.x
- **ACM/OCM**: Multiple stolostron and open-cluster-management.io dependencies
- **Go Version**: 1.25.3

## Environment Variables

E2E testing:


- `MH_NUM`: Number of managed hub clusters (default: 2)
- `MC_NUM`: Number of managed clusters (default: 1)
- `GH_NAMESPACE`: Global hub namespace (default: multicluster-global-hub)
- `VERBOSE`: Log verbosity level for E2E tests (default: 5)

Build:

- `REGISTRY`: Container registry (default: quay.io/stolostron)
- `IMAGE_TAG`: Image tag (default: latest)
- `GO_TEST`: Go test command override
