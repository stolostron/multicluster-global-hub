# Repository Guidelines

## Project Structure & Module Organization
- `agent/`, `manager/`, and `operator/` hold the main Go services; each has its own `Makefile` and Kubernetes manifests under `config/`.
- `pkg/` contains shared libraries used by multiple services; keep cross-component imports minimal to avoid the circular dependency checks enforced by `make fmt`.
- `test/` bundles unit helpers, integration suites, and e2e scripts under `test/script/`. Supporting assets, manifests, and Grafana dashboards live in `samples/` and `doc/`.
- Use `scripts/` for operational tooling (release automation, log collection) and `tools/` for pinned build utilities.

## Build, Test, and Development Commands
- `make build-manager-image` (or `build-operator-image`, `build-agent-image`) builds the respective container image defined under each component’s `Dockerfile`.
- `make unit-tests` runs Go unit suites across `pkg`, `operator`, `manager`, and `agent` using controller-runtime’s envtest assets.
- `make integration-test` exercises the multi-service flows under `test/integration/...`; pass `integration-test/agent` or similar targets for scoped runs.
- `make e2e-test-all` provisions dependencies and executes the curated scenario list declared in `test/Makefile`.
- `make fmt` or `make strict-fmt` applies `go fmt`, `gci`, and `gofumpt`, then fails if components import each other accidentally.

## Coding Style & Naming Conventions
- Go code follows standard formatting with tabs and `camelCase` identifiers for locals, `UpperCamelCase` for exported types, and `ALL_CAPS` for env vars.
- Favor `pkg/<domain>` packages for reusable logic and keep controller reconcilers under `*/controllers`.
- Run `make fmt` before committing; CI expects no diff and no cross-component import leaks.

## Testing Guidelines
- Unit and integration tests use Go’s `testing` package with `envtest`; name test files `<name>_test.go` and helper suites `<feature>_suite_test.go`.
- Tag long-running scenarios with `//go:build e2e` if you add compiled e2e helpers; keep mocks in `test/<area>/mocks`.
- E2E flows rely on the scripts in `test/script/`; ensure prerequisites (clusters, kubeconfig) are configured before invoking `make e2e-setup` or `make kessel-e2e-run`.

## Commit & Pull Request Guidelines
- Follow the Conventional Commit pattern observed in history (`fix:`, `docs:`, emoji-prefixed dependency bumps) and include issue numbers like `(#2064)` when applicable.
- Group changes by component and keep messages imperative (“Add agent status metrics”).
- Pull requests should describe scope, testing performed, and any operator upgrade steps; attach links to related issues and screenshots for UI-facing updates.
- Verify `make unit-tests` and relevant integration or e2e commands locally before requesting review; note any skipped suites in the PR description.
