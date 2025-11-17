# Global Hub New Release Workflow

Automates the complete end-to-end workflow for cutting a new Multicluster Global Hub release across all repositories.

## Quick Start

### Important: RELEASE_BRANCH is Required

**All commands MUST specify the RELEASE_BRANCH environment variable.** The skill does not auto-detect release versions.

### Using Claude Code

Ask Claude with explicit version:

```
RELEASE_BRANCH=release-2.17 cut a new Global Hub release
```

or

```
Cut Global Hub release for ACM release-2.17 (set RELEASE_BRANCH=release-2.17)
```

### Using Scripts Directly

**RELEASE_BRANCH is REQUIRED for all commands:**

```bash
# Interactive mode - choose which repos to update
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh

# Update all repositories
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all

# Update specific repositories
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh 1,2    # Only main repo and openshift/release
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh 3,4,5,6  # Only bundle, catalog, grafana, postgres

# CREATE_BRANCHES mode - create new branches and push directly
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/cut-release.sh all

# UPDATE mode (default) - create PRs to update existing branches
CREATE_BRANCHES=false RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all
```

## Version Mapping

The skill automatically calculates all version numbers based on ACM version:

| ACM Version | Release Branch | Global Hub Version | Bundle/Catalog Branch | OCP Versions |
|-------------|----------------|--------------------|----------------------|--------------|
| 2.14        | release-2.14   | v1.5.0             | release-1.5          | 4.15 - 4.19  |
| 2.15        | release-2.15   | v1.6.0             | release-1.6          | 4.16 - 4.20  |
| 2.16        | release-2.16   | v1.7.0             | release-1.7          | 4.17 - 4.21  |
| 2.17        | release-2.17   | v1.8.0             | release-1.8          | 4.18 - 4.22  |
| 2.18        | release-2.18   | v1.9.0             | release-1.9          | 4.19 - 4.23  |

**Formulas**:
- Global Hub version: v1.X.0 where X = ACM_MINOR - 9
- OCP versions: 4.(GH_MINOR + 10) to 4.(GH_MINOR + 14)

## Repository Structure

The skill manages 6 repositories:

### 1. Multicluster Global Hub (Main Repository)
**Script**: `01-multicluster-global-hub.sh`

**What it does**:
- Creates release branch (e.g., release-2.17) OR updates existing branch
- Updates `.tekton/` pipelinesascode configurations
- Updates `Containerfile.*` version references
- **Creates PR** (UPDATE mode) or **pushes directly** (CUT mode)

**Example**:
```bash
# Update existing release
RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/01-multicluster-global-hub.sh

# Create new release
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/01-multicluster-global-hub.sh
```

### 2. OpenShift Release (CI Configuration)
**Script**: `02-openshift-release.sh`

**What it does**:
- Updates main branch CI configuration (promotion, fast-forward)
- Creates new release pipeline configuration
- Auto-generates presubmit/postsubmit jobs using `make update`
- **Creates PR** with all CI changes

**Example**:
```bash
RELEASE_BRANCH=release-2.17 ./scripts/02-openshift-release.sh
```

### 3. Operator Bundle
**Script**: `03-bundle.sh`

**What it does**:
- Creates bundle branch (e.g., release-1.8) OR updates existing branch
- Updates `images_digest_mirror_set.yaml`
- Renames and updates tekton pipelines
- Updates bundle image labels
- Updates `konflux-patch.sh`
- **Creates PR** (UPDATE mode) or **pushes directly** (CUT mode)
- **Creates cleanup PR** to remove GitHub Actions from old release

**Example**:
```bash
# Update existing release
RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/03-bundle.sh

# Create new release
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/03-bundle.sh
```

### 4. Operator Catalog
**Script**: `04-catalog.sh`

**What it does**:
- Creates catalog branch (e.g., release-1.8) OR updates existing branch
- Updates `images-mirror-set.yaml`
- **Adds** new OCP version pipelines (e.g., OCP 4.22)
- **Removes** old OCP version pipelines (e.g., OCP 4.17)
- Updates existing OCP pipelines (4.18-4.21)
- **Creates PR** (UPDATE mode) or **pushes directly** (CUT mode)
- **Creates cleanup PR** to remove GitHub Actions from old release

**Example**:
```bash
# Update existing release
RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/04-catalog.sh

# Create new release
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/04-catalog.sh
```

### 5. Glo-Grafana
**Script**: `05-grafana.sh`

**What it does**:
- Creates grafana branch (e.g., release-1.8) OR updates existing branch
- Renames and updates tekton pipelines
- **Creates PR** (UPDATE mode) or **pushes directly** (CUT mode)

**Example**:
```bash
# Update existing release
RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/05-grafana.sh

# Create new release
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/05-grafana.sh
```

### 6. Postgres Exporter
**Script**: `06-postgres-exporter.sh`

**What it does**:
- Creates postgres_exporter branch (same as ACM, e.g., release-2.17) OR updates existing branch
- Renames and updates tekton pipelines
- **Creates PR** (UPDATE mode) or **pushes directly** (CUT mode)

**Example**:
```bash
# Update existing release
RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/06-postgres-exporter.sh

# Create new release
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/06-postgres-exporter.sh
```

## Environment Variables

The main orchestration script (`cut-release.sh`) calculates and exports these variables:

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `RELEASE_BRANCH` | ACM release branch | `release-2.17` | **YES** - Must be explicitly set |
| `CREATE_BRANCHES` | Operating mode (true/false) | `false` | Optional (default: false) |
| `GITHUB_USER` | GitHub username for PRs | `yanmxa` | Optional (auto-detected from git) |
| `ACM_VERSION` | ACM version number | `2.17` | Auto-calculated |
| `GH_VERSION` | Global Hub version | `v1.8.0` | Auto-calculated |
| `GH_VERSION_SHORT` | Short Global Hub version | `1.8` | Auto-calculated |
| `BUNDLE_BRANCH` | Bundle release branch | `release-1.8` | Auto-calculated |
| `BUNDLE_TAG` | Bundle image tag | `globalhub-1-8` | Auto-calculated |
| `CATALOG_BRANCH` | Catalog release branch | `release-1.8` | Auto-calculated |
| `CATALOG_TAG` | Catalog image tag | `globalhub-1-8` | Auto-calculated |
| `GRAFANA_BRANCH` | Grafana release branch | `release-1.8` | Auto-calculated |
| `GRAFANA_TAG` | Grafana tag | `globalhub-1-8` | Auto-calculated |
| `POSTGRES_TAG` | Postgres image tag | `globalhub-1-8` | Auto-calculated |
| `OCP_MIN` | Minimum OCP version number | `417` | Auto-calculated |
| `OCP_MAX` | Maximum OCP version number | `421` | Auto-calculated |
| `OPENSHIFT_RELEASE_PATH` | Path to openshift/release clone | `/tmp/openshift-release` | Optional |
| `WORK_DIR` | Working directory for repos | `/tmp/globalhub-release-repos` | Optional |

**Operating Modes**:
- `CREATE_BRANCHES=true`: Create new release branches and push directly to upstream
- `CREATE_BRANCHES=false` (default): Update existing release branches via pull requests

Individual scripts can use these variables instead of calculating them.

## Prerequisites

1. **Git configured** with your GitHub credentials
2. **GitHub CLI (`gh`)** authenticated with your account
3. **Fork** of https://github.com/openshift/release (for script 02)
4. **Container engine** (Docker or Podman) running (for script 02's `make update`)
5. **Write access** to all stolostron repositories

## Platform Support

All scripts are compatible with:
- **macOS** (ARM/Apple Silicon and Intel)
- **Linux** (x86_64)

Scripts automatically detect the OS and use appropriate `sed` syntax.

## Workflow Examples

### Example 1: Full Release (All Repositories)

```bash
$ RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all

Using specified release: release-2.17

📊 Version Information
================================================
   Mode:        UPDATE (PR only)
   GitHub User: yanmxa
   ACM:         release-2.17
   Global Hub:  release-1.8
   Bundle:      release-1.8
   Catalog:     release-1.8
   OCP:         4.18 - 4.22
================================================

Mode: Update all repositories

🚀 Starting Release Workflow
================================================

────────────────────────────────────────────────
📦 [1/6] multicluster-global-hub
────────────────────────────────────────────────
   Main repository with operator, manager, and agent

🚀 Multicluster Global Hub Release Branch Creation
...
✅ multicluster-global-hub completed successfully

────────────────────────────────────────────────
📦 [2/6] openshift/release
────────────────────────────────────────────────
   OpenShift CI configuration
...
✅ openshift/release completed successfully

... (continues for all 6 repositories)

================================================
📋 Release Workflow Summary
================================================

Version:
   ACM: 2.17
   Global Hub: v1.8.0

Results:
   Total: 6
   ✅ Completed: 6
   ❌ Failed: 0

================================================
🎉 All selected repositories updated successfully!

📝 Next Steps:
   1. Review and merge created PRs
   2. Verify all release branches
   3. Update konflux-release-data (manual)
```

### Example 2: Create New Release (CREATE_BRANCHES Mode)

```bash
$ CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./scripts/cut-release.sh all

Using specified release: release-2.18

📊 Version Information
================================================
   Mode:        CREATE_BRANCHES (create branches)
   GitHub User: yanmxa
   ACM:         release-2.18
   Global Hub:  release-1.9
   Bundle:      release-1.9
   Catalog:     release-1.9
   OCP:         4.19 - 4.23
================================================

Mode: Update all repositories

... (creates new branches and pushes directly to upstream)
```

### Example 3: Update Only Specific Repositories

```bash
$ RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh 1,2

Mode: Update selected repositories: 1 2

... (updates only multicluster-global-hub and openshift/release)
```

### Example 4: Interactive Mode

```bash
$ RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh

Available repositories:

   [1] multicluster-global-hub    Main repository with operator, manager, and agent
   [2] openshift/release           OpenShift CI configuration
   [3] operator-bundle             Operator bundle manifests
   [4] operator-catalog            Operator catalog for OCP versions
   [5] glo-grafana                 Grafana dashboards
   [6] postgres_exporter           Postgres exporter

Select repositories to update:
   Enter numbers separated by commas (e.g., 1,2,3)
   Or press Enter to update all repositories

Selection: 3,4

Updating selected repositories: 3 4
...
```

### Example 5: Standalone Script Execution

```bash
# Run only the bundle script (UPDATE mode)
$ RELEASE_BRANCH=release-2.17 CREATE_BRANCHES=false ./scripts/03-bundle.sh

# Run only the catalog script (CREATE_BRANCHES mode)
$ RELEASE_BRANCH=release-2.18 CREATE_BRANCHES=true ./scripts/04-catalog.sh
```

## Output Files and PRs Created

After running the complete workflow:

### Pull Requests Created

1. **multicluster-global-hub**: Version bump PR to main branch
2. **openshift/release**: CI configuration PR
3. **operator-bundle**: Bundle update PR to main branch
4. **operator-catalog**:
   - New release configuration PR to main branch
   - Cleanup PR to old release branch (removes GitHub Actions)

Total: **5 PRs** to review and merge

### Branches Created

1. `multicluster-global-hub`: `release-2.17`
2. `operator-bundle`: `release-1.8`
3. `operator-catalog`: `release-1.8`
4. `glo-grafana`: `release-1.8`
5. `postgres_exporter`: `release-2.17`

Total: **5 new release branches**

## Troubleshooting

### Container Engine Not Running (Script 02)

**Error**: `❌ Error: No container engine found!`

**Solution**:
- **Docker**: Start Docker Desktop application
- **Podman**: `podman machine start`

### Fork Not Found (Script 02)

**Error**: `Fork not found`

**Solution**:
1. Go to https://github.com/openshift/release
2. Click "Fork" button
3. Run the script again

### GitHub CLI Not Authenticated

**Error**: `gh: command not found` or authentication errors

**Solution**:
```bash
brew install gh  # or appropriate package manager
gh auth login
```

### Script Fails Mid-Way

The orchestration script asks if you want to continue after each failure:

```
❌ operator-bundle failed

⚠️  Continue with remaining repositories? (y/n)
```

You can:
- Type `y` to continue with remaining repos
- Type `n` to abort and fix the issue
- Re-run the script with only the failed repo number

## Manual Steps Still Required

After running the automated scripts, you still need to manually:

1. **Merge all created PRs**
   - Review changes carefully
   - Ensure CI passes
   - Merge in appropriate order

2. **Update konflux-release-data repository**
   - Add new release configuration
   - (Not automated by this skill)

3. **Verify release branches**
   - Check all branches were created correctly
   - Verify version numbers match expectations

## Release Update Checklist

### Quick Summary

For **release-2.17 → Global Hub 1.8 → OCP 4.18-4.22**:

| Repository | Branch | Files Updated | Key Changes |
|------------|--------|---------------|-------------|
| **multicluster-global-hub** | `release-2.17` | `.tekton/`, `Containerfile.*`, `.github/workflows/go.yml`, `renovate.json`, `operator/Makefile`, CSV | Create new tekton pipelines, remove old ones, update versions |
| **openshift/release** | `master` | `ci-operator/config/`, `ci-operator/jobs/` | Add new release CI config, auto-generate jobs |
| **operator-bundle** | `release-1.8` | `images_digest_mirror_set.yaml`, `.tekton/`, `konflux-patch.sh`, `bundle/manifests/` | Update image refs, rename pipelines |
| **operator-catalog** | `release-1.8` | `images-mirror-set.yaml`, `v4.XX/Containerfile.catalog`, `.tekton/`, `catalog-template-current.json` | Add new OCP v4.22, remove v4.17, update all Containerfiles |
| **glo-grafana** | `release-1.8` | `.tekton/` | Rename and update pipelines |
| **postgres_exporter** | `release-2.17` | `.tekton/` | Rename and update pipelines |

### Detailed File Changes

<details>
<summary><b>1. multicluster-global-hub</b> (PR to main)</summary>

**Created**:
- `.tekton/*-globalhub-1-8-*.yaml` (6 new pipeline files)

**Removed**:
- `.tekton/*-globalhub-1-7-*.yaml` (6 old pipeline files)

**Updated**:
- `agent/Containerfile.agent`, `manager/Containerfile.manager`, `operator/Containerfile.operator`
  - `LABEL version="release-1.8"`
- `.github/workflows/go.yml`
  - Bundle branch: `release-1.7` → `release-1.8`
- `renovate.json`
  - `baseBranches: ["main", "release-2.16", "release-2.15", "release-2.14"]`
- `operator/Makefile`
  - VERSION, CHANNELS, DEFAULT_CHANNEL → `1.8`
- `operator/config/manifests/bases/multicluster-global-hub-operator.clusterserviceversion.yaml`
  - `olm.skipRange`, ACM docs URL, `maturity`
- `operator/bundle/` (regenerated if needed)

</details>

<details>
<summary><b>2. openshift/release</b> (PR to master)</summary>

**Created**:
- `ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-2.17.yaml`
- `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-2.17-presubmits.yaml` (auto-generated)
- `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-2.17-postsubmits.yaml` (auto-generated)

</details>

<details>
<summary><b>3. operator-bundle</b> (PR to release-1.8)</summary>

**Updated**:
- `images_digest_mirror_set.yaml`
  - All image refs: `globalhub-1-7` → `globalhub-1-8`
- `.tekton/*-globalhub-1-8-*.yaml` (renamed/updated)
- `konflux-patch.sh`
  - Version refs: `1.7` → `1.8`
- `bundle/manifests/*.yaml`
  - Channel labels: `release-1.8`

</details>

<details>
<summary><b>4. operator-catalog</b> (PR to main + PR to release-1.8)</summary>

**Created** (on main):
- `v4.22/catalog/multicluster-global-hub-operator-rh/catalog.json`

**Removed** (on release branch):
- `v4.17/` directory and pipelines

**Created** (on release branch):
- `v4.22/Containerfile.catalog`
- `.tekton/*-v422-globalhub-1-8-*.yaml` (2 new pipelines)

**Updated** (on release branch):
- `images-mirror-set.yaml`
  - `globalhub-1-7` → `globalhub-1-8`
- `catalog-template-current.json`
  - Channel, version, skipRange → `1.8`
- `v4.18/Containerfile.catalog`, `v4.19/`, `v4.20/`, `v4.21/`, `v4.22/`
  - Bundle ref: `globalhub-1-7` → `globalhub-1-8`
- `.tekton/*-v4{18,19,20,21}-globalhub-1-8-*.yaml` (8 updated pipelines)

</details>

<details>
<summary><b>5. glo-grafana</b> (PR to release-1.8)</summary>

**Updated**:
- `.tekton/grafana-globalhub-1-8-*.yaml` (renamed/updated)

</details>

<details>
<summary><b>6. postgres_exporter</b> (PR to release-2.17)</summary>

**Updated**:
- `.tekton/postgres-exporter-globalhub-1-8-*.yaml` (renamed/updated)

</details>

---

## Related Documentation

- [RELEASE_CHECKLIST.md](../../../RELEASE_CHECKLIST.md) - Complete release checklist
- [SKILL.md](SKILL.md) - Technical details about the skill implementation

## Contributing

To improve this skill:

1. Edit individual scripts in `.claude/skills/cut-release/scripts/`
2. Test with a dry run or on a test repository
3. Update this README if behavior changes
4. Commit and share with the team

## License

This skill is part of the Multicluster Global Hub project.
