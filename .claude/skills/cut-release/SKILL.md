---
name: cut-release
description: Automate complete Multicluster Global Hub release workflow across 6 repositories including branch creation, OpenShift CI configuration, catalog OCP version management, bundle updates, and PR creation. Use when cutting new releases (e.g., release-2.16, release-2.17). Keywords: release branch, global hub, openshift/release, catalog, bundle, CI configuration, OCP versions.
allowed-tools: [Read, Write, Bash, Glob, Grep]
---

# Global Hub Release Workflow (Cut Release)

Automates the complete end-to-end workflow for cutting a new Multicluster Global Hub release across all 6 repositories with full automation.

## When to Use This Skill

- Cutting a new Global Hub release (e.g., release-2.16, release-2.17)
- Setting up complete release infrastructure across all repositories
- Automating the entire release checklist workflow
- Creating coordinated PRs and branches for new releases

## Version Mapping (ACM to Global Hub)

The skill uses ACM release branch names as the primary input and automatically calculates the corresponding Global Hub version.

### Version Mapping Table

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

**Example**: ACM release-2.17 → Global Hub v1.8.0 → Bundle/Catalog release-1.8

## Repository Workflow

The skill manages releases across 6 repositories, each with its own dedicated script:

### Repository 1: multicluster-global-hub (Script 01)

**Repository**: [stolostron/multicluster-global-hub](https://github.com/stolostron/multicluster-global-hub)

**What it does**:
- Creates ACM release branch (e.g., release-2.17)
- Updates `.tekton/` pipelinesascode configurations
- Updates `Containerfile.*` version references
- Creates PR to bump version in main branch for next development cycle

**Outputs**: 1 PR (version bump to main)

### Repository 2: openshift/release (Script 02)

**Repository**: [openshift/release](https://github.com/openshift/release)

**What it does**:
- Updates main branch CI configuration (promotion, fast-forward)
- Creates new release pipeline configuration
- Auto-generates presubmit/postsubmit jobs using `make update`
- Validates container engine availability (Docker or Podman)
- Creates PR with all CI changes

**Outputs**: 1 PR (CI configuration)

### Repository 3: operator-bundle (Script 03)

**Repository**: [stolostron/multicluster-global-hub-operator-bundle](https://github.com/stolostron/multicluster-global-hub-operator-bundle)

**What it does**:
- Creates bundle branch using Global Hub version (e.g., release-1.8 from v1.8.0)
- Updates `images_digest_mirror_set.yaml` with new image tags
- Renames and updates tekton pipelines (pull-request and push)
- Updates bundle image labels to new version
- Updates `konflux-patch.sh` image references
- Creates PR with all bundle changes

**Outputs**: 1 PR (bundle updates to main)

### Repository 4: operator-catalog (Script 04)

**Repository**: [stolostron/multicluster-global-hub-operator-catalog](https://github.com/stolostron/multicluster-global-hub-operator-catalog)

**What it does**:
- Creates catalog branch using Global Hub version (e.g., release-1.8)
- Updates `images-mirror-set.yaml` with new image tags
- **Adds** new OCP version pipelines (e.g., OCP 4.21 for release-1.8)
- **Removes** old OCP version pipelines (e.g., OCP 4.16)
- Updates existing OCP version pipelines (4.17-4.20)
- Updates `README.md` with new version information
- Updates GitHub Actions workflow for new release branch
- Creates **2 PRs**:
  - Main PR: New release configuration to main branch
  - Cleanup PR: Remove GitHub Actions from old release branch

**OCP Version Formula**: OCP_MIN = 4.(10 + GH_MINOR), OCP_MAX = OCP_MIN + 4

**Outputs**: 2 PRs (main release + cleanup)

### Repository 5: glo-grafana (Script 05)

**Repository**: [stolostron/glo-grafana](https://github.com/stolostron/glo-grafana)

**What it does**:
- Creates grafana branch using Global Hub version (e.g., release-1.8)
- Renames and updates tekton pipelines (pull-request and push)
- Updates branch references in pipeline files

**Outputs**: Branch creation only (no PR)

### Repository 6: postgres_exporter (Script 06)

**Repository**: [stolostron/postgres_exporter](https://github.com/stolostron/postgres_exporter)

**What it does**:
- Creates postgres_exporter branch using ACM version (e.g., release-2.17)
- Renames and updates tekton pipelines (pull-request and push)
- Updates branch references in pipeline files

**Outputs**: Branch creation only (no PR)

## Script Organization

```
.claude/skills/cut-release/
├── SKILL.md                          # This file
├── README.md                         # User documentation
└── scripts/
    ├── cut-release.sh                # Main orchestration script
    ├── 01-multicluster-global-hub.sh # Main repo (creates version bump PR)
    ├── 02-openshift-release.sh       # CI config (creates PR)
    ├── 03-bundle.sh                  # Bundle (creates PR)
    ├── 04-catalog.sh                 # Catalog (creates 2 PRs)
    ├── 05-grafana.sh                 # Grafana (branch only)
    └── 06-postgres-exporter.sh       # Postgres exporter (branch only)
```

## Execution Modes

The main orchestration script supports three modes:

### 1. Interactive Mode (Default)
```bash
./scripts/cut-release.sh
```
Prompts user to select which repositories to update (1-6, comma-separated, or all).

### 2. All Repositories Mode
```bash
./scripts/cut-release.sh all
```
Updates all 6 repositories in sequence.

### 3. Selective Mode
```bash
./scripts/cut-release.sh 1,2,3    # Update specific repos
./scripts/cut-release.sh 3,4      # Update only bundle and catalog
```

### 4. Standalone Script Execution
Each script can be run independently:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/03-bundle.sh
```

## Environment Variables

The main orchestration script calculates and exports these variables for child scripts:

| Variable | Description | Example |
|----------|-------------|---------|
| `RELEASE_BRANCH` | ACM release branch | `release-2.17` |
| `ACM_VERSION` | ACM version number | `2.17` |
| `GH_VERSION` | Global Hub version | `v1.8.0` |
| `GH_VERSION_SHORT` | Short Global Hub version | `1.8` |
| `BUNDLE_BRANCH` | Bundle release branch | `release-1.8` |
| `BUNDLE_TAG` | Bundle image tag | `globalhub-1-8` |
| `CATALOG_BRANCH` | Catalog release branch | `release-1.8` |
| `CATALOG_TAG` | Catalog image tag | `globalhub-1-8` |
| `GRAFANA_BRANCH` | Grafana release branch | `release-1.8` |
| `POSTGRES_TAG` | Postgres image tag | `globalhub-1-8` |
| `OPENSHIFT_RELEASE_PATH` | Path to openshift/release clone | `/tmp/openshift-release` |

Individual scripts can be run standalone by providing these variables.

## Platform Compatibility

All scripts are compatible with:
- **macOS** (ARM/Apple Silicon and Intel) - Uses `sed -i ""` syntax
- **Linux** (x86_64) - Uses `sed -i` syntax

Scripts automatically detect the OS and use the appropriate `sed` syntax.

## Prerequisites

1. **Git configured** with your GitHub credentials
2. **GitHub CLI (`gh`)** authenticated with your account
3. **Fork** of https://github.com/openshift/release (for script 02)
4. **Container engine** (Docker or Podman) running (for script 02's `make update`)
5. **Write access** to all stolostron repositories

## Workflow Summary

When running the complete workflow (`cut-release.sh all`):

### Phase 1: Main Repository (Script 01)
- Detects latest release and calculates next version
- Creates release branch from main
- Updates .tekton/ and Containerfile.*
- Creates version bump PR to main branch

### Phase 2: OpenShift CI (Script 02)
- Validates container engine availability
- Updates CI configurations for new release
- Auto-generates job configurations
- Creates PR to openshift/release

### Phase 3: Bundle (Script 03)
- Creates bundle release branch
- Updates all bundle configurations and tekton pipelines
- Creates PR to main branch

### Phase 4: Catalog (Script 04)
- Creates catalog release branch
- Manages OCP version lifecycle (add newest, remove oldest)
- Updates all catalog configurations
- Creates 2 PRs (main release + old branch cleanup)

### Phase 5: Grafana (Script 05)
- Creates grafana release branch
- Updates tekton pipelines

### Phase 6: Postgres Exporter (Script 06)
- Creates postgres_exporter release branch
- Updates tekton pipelines

## Total Output

After running all 6 scripts:

**Pull Requests Created**: 5 PRs
1. Main repo: Version bump PR
2. OpenShift CI: CI configuration PR
3. Bundle: Bundle update PR
4. Catalog: New release configuration PR
5. Catalog: Cleanup PR for old branch

**Branches Created**: 5 branches
1. `multicluster-global-hub`: `release-2.17`
2. `operator-bundle`: `release-1.8`
3. `operator-catalog`: `release-1.8`
4. `glo-grafana`: `release-1.8`
5. `postgres_exporter`: `release-2.17`

## Error Handling

The orchestration script includes error handling:
- Each script can fail independently
- User is prompted to continue or abort after failures
- Final summary shows completed vs failed repositories
- Exit code indicates overall success/failure

## Example Usage

### Auto-detect and update all repositories:
```bash
./.claude/skills/cut-release/scripts/cut-release.sh all
```

### Specify version explicitly:
```bash
RELEASE_BRANCH="release-2.17" ./.claude/skills/cut-release/scripts/cut-release.sh all
```

### Interactive selection:
```bash
./.claude/skills/cut-release/scripts/cut-release.sh
# Then select: 3,4 (to update only bundle and catalog)
```

### Standalone script execution:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/04-catalog.sh
```

## Output Format

The skill provides clear progress indicators:

```
🔍 Detecting latest release branch...
   Latest release: release-2.16
   Next release: release-2.17

📊 Version Information
================================================
   ACM Release:          release-2.17 (2.17)
   Global Hub Version:   v1.8.0
   Bundle Branch:        release-1.8
   Catalog Branch:       release-1.8
   Supported OCP:        4.17 - 4.21
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

## Manual Steps Still Required

After running the automated scripts:

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

## Related Documentation

- [README.md](README.md) - Complete user documentation with examples
- [RELEASE_CHECKLIST.md](../../../RELEASE_CHECKLIST.md) - Original manual checklist

## Automation Coverage

This skill automates the following from RELEASE_CHECKLIST.md:

- ✅ Create new release branches (all 6 repositories)
- ✅ Update pipelinesascode configurations (.tekton/)
- ✅ Update Containerfile.* versions
- ✅ Update openshift/release configurations
- ✅ Auto-generate CI job configurations
- ✅ Update bundle manifests and image references
- ✅ Manage catalog OCP version lifecycle
- ✅ Update catalog README and image mirror sets
- ✅ Update GitHub Actions workflows
- ✅ Create all necessary PRs
- ⚠️  Manual: Update konflux-release-data repository
- ⚠️  Manual: Merge PRs after review
