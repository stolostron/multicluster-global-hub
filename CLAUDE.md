# CLAUDE.md — multicluster-global-hub

AI assistant context for **Multicluster Global Hub** — a Kubernetes operator-based system for managing ACM/OCM at very high scale across multiple regional hub clusters.

Onboarded per [Fleet Engineering Agentic SDLC — repo onboarding](https://github.com/OpenShift-Fleet/agentic-sdlc/blob/main/practices/repo-onboarding.md). Day-to-day workflow: [ai-dev-workflow.md](https://github.com/OpenShift-Fleet/agentic-sdlc/blob/main/practices/ai-dev-workflow.md).

---

## Build, Test, and Lint Commands

```bash
# Vendor dependencies (required before image builds)
make vendor

# Format and lint
make fmt
make strict-fmt          # gci + gofumpt (CI format job)

# Update dependencies
make tidy
make vendor

# Build component binaries
cd operator && make
cd manager && make
cd agent && make

# Build and push container images
make build-operator-image push-operator-image REGISTRY=<registry> IMAGE_TAG=<tag>
make build-manager-image push-manager-image REGISTRY=<registry> IMAGE_TAG=<tag>
make build-agent-image push-agent-image REGISTRY=<registry> IMAGE_TAG=<tag>

# Deploy operator and install a Global Hub instance
make deploy-operator
kubectl apply -k operator/config/samples/
make undeploy-operator

# Unit tests (downloads envtest binaries to /tmp/cr-tests-bin on first run)
make unit-tests
make unit-tests-operator
make unit-tests-manager
make unit-tests-agent
make unit-tests-pkg

# Integration tests
make integration-test
make integration-test/operator
make integration-test/manager
make integration-test/agent

# E2E tests (creates KinD clusters)
make e2e-setup
make e2e-test-cluster
make e2e-test-local-agent
make e2e-test-localpolicy
make e2e-test-grafana
make e2e-test-all
make e2e-cleanup
make e2e-test-localpolicy VERBOSE=9   # verbose E2E output

# Operator API / bundle generation
cd operator
make generate    # DeepCopy, etc.
make manifests   # CRDs, RBAC
make bundle      # OLM bundle

# E2E log collection
make e2e-log/operator
make e2e-log/manager
make e2e-log/grafana
make e2e-log/agent
```

> Unit and integration tests use controller-runtime's envtest harness. `KUBEBUILDER_ASSETS` is set automatically by the Makefile via `setup-envtest`.
>
> `make fmt` enforces import boundaries between `pkg/`, `operator/`, `manager/`, and `agent/` — see [Code Formatting Rules](#code-formatting-rules) below.

---

## Repo Layout

```text
operator/             OLM operator — deploys manager, agent, Kafka, Postgres, Grafana
  api/                CRD type definitions (MulticlusterGlobalHub, Agent, Migration)
  pkg/controllers/    Operator reconcilers (storage, kafka, grafana, agent, migration)
  config/             Kustomize manifests (CRDs, RBAC, manager, samples)
manager/              Runs on global hub — syncs spec/status via PostgreSQL and Kafka
  pkg/spec/           Spec sync: controllers → DB → Kafka → managed hubs
  pkg/status/         Status sync: Kafka → conflator → DB
  pkg/controllers/    Migration, backup controllers
  pkg/processes/      Cronjobs (compliance summarization, managed hub management)
  pkg/restapis/       REST APIs (clusters, policies, subscriptions)
agent/                Runs on managed hub clusters — applies spec, reports status
  pkg/spec/           Applies resources from global hub to managed hub
  pkg/status/         Reports managed-hub status to global hub via Kafka/Inventory API
  pkg/controllers/    Inventory API controllers
pkg/                  Shared code (transport, database, bundle, constants, utils)
test/                 Integration and E2E test scripts and configs
  script/             KinD E2E setup/run/cleanup scripts
doc/                  Human-facing product documentation and architecture diagrams
samples/              Example CRs and deployment manifests
tools/                Helper scripts (kafka config generation, etc.)
```

---

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for CRDs, spec/status data flow, PostgreSQL schema layout, Kafka topics, controller reconciliation, namespace layout, and migration/DR patterns.

Additional product docs: [doc/README.md](doc/README.md), [doc/how_global_hub_works.md](doc/how_global_hub_works.md).

---

## Code Formatting Rules

The `make fmt` target enforces import dependency rules:

- `pkg/` must NOT import from `agent/`, `operator/`, or `manager/`
- `operator/` must NOT import from `agent/` or `manager/`
- `agent/` must NOT import from `manager/` or `operator/` (except `operator/api`)
- `manager/` must NOT import from `agent/` or `operator/` (except `operator/api`)

Only shared code should live in `pkg/`.

---

## Personal Config

Read `.claude/user.local.md` at the start of any task that needs an assignee, email, or project key. If the file does not exist, fall back to Claude memory (`user-config`), then placeholders. Run `make personalize` in [OpenShift-Fleet/agentic-sdlc](https://github.com/OpenShift-Fleet/agentic-sdlc) to generate it.

Repo-local skills live under `.claude/skills/` (e.g. `new-release` for z-stream cut workflows).

---

## Fleet Engineering Skills

Use the Fleet plugin in Claude Code (`make install-claude` in agentic-sdlc) when available. In Cursor or other agents without the plugin, fetch and apply the relevant skill URL when the task matches its domain.

| Skill | When to use |
|---|---|
| [start-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/sdlc/start-work/SKILL.md) | Begin work on a Jira ticket — creates sub-task, transitions status |
| [finish-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/sdlc/finish-work/SKILL.md) | Commit, push, open PR, update Jira |
| [pr-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/sdlc/pr-review/SKILL.md) | Review a GitHub PR with worktree isolation and inline comments |
| [pr-fix](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/sdlc/pr-fix/SKILL.md) | Fix blocked PRs: merge conflicts, CI failures, review comments |
| [jira-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira/jira-specialist/SKILL.md) | Triage, search, link, or transition Jira issues |
| [bug-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira/bug-specialist/SKILL.md) | Create a well-structured bug report with reproduction steps |
| [story-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira/story-specialist/SKILL.md) | Create a user story with acceptance criteria |
| [spike-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira/spike-specialist/SKILL.md) | Time-boxed research and PoC tickets |

---

## Key Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| Go | 1.26.3 | Toolchain |
| Kubernetes | v0.33.x | client-go, api, apimachinery |
| controller-runtime | v0.21.x | Operator framework |
| IBM Sarama | v1.46.x | Kafka client |
| Confluent Kafka Go | v2.14.x | Kafka client (alternative) |
| CloudEvents SDK | v2.16.x | Status transport bundles |
| GORM / pgx | v1.30.x / v5.7.x | PostgreSQL access |

---

## Environment Variables

**E2E testing:**

| Variable | Default | Purpose |
|---|---|---|
| `MH_NUM` | 2 | Managed hub KinD clusters |
| `MC_NUM` | 1 | Managed clusters per hub |
| `GH_NAMESPACE` | `multicluster-global-hub` | Global hub namespace |
| `VERBOSE` | 5 | E2E script log level |

**Build:**

| Variable | Default | Purpose |
|---|---|---|
| `REGISTRY` | `quay.io/stolostron` | Container registry |
| `IMAGE_TAG` | `latest` | Image tag |
| `GO_TEST` | `go test -v` | Go test command override |
