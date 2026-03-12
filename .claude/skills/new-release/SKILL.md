---
name: new-release
description: "Automate complete Multicluster Global Hub release workflow across 6 repositories including branch creation, OpenShift CI configuration, catalog OCP version management, bundle updates, and PR creation. REQUIRES explicit RELEASE_BRANCH specification (e.g., RELEASE_BRANCH=release-2.17). Supports two modes - CREATE_BRANCHES=true (create branches) or false (update via PR). Keywords: release branch, global hub, openshift/release, catalog, bundle, CI configuration, OCP versions."
allowed-tools: ["Read", "Write", "Bash", "Glob", "Grep"]
---

# Global Hub New Release Workflow

Automates the complete end-to-end workflow for cutting a new Multicluster Global Hub release across all 6 repositories with full automation.

## Execution Instructions for Claude

**CRITICAL - Synchronous Execution Required**:

1. **Run Synchronously**: Execute the cut-release.sh script using the Bash tool WITHOUT the `run_in_background` parameter. The script must run synchronously to completion with a 600000ms (10 minute) timeout.

2. **No BashOutput Calls**: DO NOT use the BashOutput tool. The command runs synchronously and returns complete output when finished.

## When to Use This Skill

- Cutting a new Global Hub release (e.g., release-2.16, release-2.17)
- Updating an existing release branch with new configurations
- Setting up complete release infrastructure across all repositories
- Automating the entire release checklist workflow
- Creating coordinated PRs and branches for new releases

## Important Requirements

**RELEASE_BRANCH must be explicitly specified** - The skill does NOT auto-detect release versions. You must provide the exact release branch name.

**Two Operating Modes**:
- `CREATE_BRANCHES=true`: Create new release branches and push directly to upstream (for initial release creation)
- `CREATE_BRANCHES=false` (default): Update existing release branches via PR (for updating current releases)

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

The skill manages releases across 6 repositories, each with its own dedicated script.

> **Version example below uses release-2.17 / Global Hub v1.8.0 (globalhub-1-8)**

---

### Repository 1: multicluster-global-hub (Script 01)

**Repository**: [stolostron/multicluster-global-hub](https://github.com/stolostron/multicluster-global-hub)

**What it does** (3-part workflow):
1. **Update main branch**: Creates new `.tekton/` configuration files for new version (e.g., globalhub-1-8) with `target_branch="main"`, updates `Containerfile.*` version labels
2. **Create PR to main**: Creates PR to upstream main with new release configurations
3. **Update previous release**: Updates previous release branch's `.tekton/` files to set `target_branch` to the previous release branch (e.g., globalhub-1-7 files updated to `target_branch="release-2.16"`)

**Key behavior**:
- New `.tekton/` files (e.g., globalhub-1-8) created by **copying** old files (globalhub-1-7), not renaming
- Old `.tekton/` files (globalhub-1-7) remain on main with `target_branch="main"`
- Previous release branch gets updated so its files point to itself (not main)
- Ensures continuous file naming progression: globalhub-1-6 → globalhub-1-7 → globalhub-1-8
- The new tekton files on `main` will be automatically synced to the current release branch (`release-2.17`) after the PR is merged — **no separate PR needed for the current release branch**

**Expected PRs**: 2

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to main | `main` | New `.tekton/globalhub-1-8-*.yaml` files; old globalhub-1-7 files removed; Containerfile version updated; Makefile VERSION/CHANNELS updated | Check `.tekton/` has globalhub-1-8 files with `target_branch=main` (pull-request) and `target_branch=release-2.17` (push) |
| PR to prev release | `release-2.16` | globalhub-1-7 `pull-request.yaml` target_branch changed from `main` → `release-2.16` | Verify `*pull-request.yaml` has `target_branch=release-2.16` |

---

### Repository 2: openshift/release (Script 02)

**Repository**: [openshift/release](https://github.com/openshift/release)

**What it does**:
- Updates main branch CI configuration (promotion, fast-forward)
- Creates new release pipeline configuration by copying from previous release
- Replaces version strings: ACM branch, version number, image prefix, GH version tag
- Auto-generates presubmit/postsubmit jobs using `make update` + `make jobs`
- Creates PR with all CI changes

**Expected PRs**: 1

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to main | `main` | `ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-2.17.yaml` created; main config updated with new release references | Check config file exists and contains `branch: release-2.17`, `name: "2.17"`, `release-218` prefix |

---

### Repository 3: operator-bundle (Script 03)

**Repository**: [stolostron/multicluster-global-hub-operator-bundle](https://github.com/stolostron/multicluster-global-hub-operator-bundle)

**What it does**:
- Checks out release branch (e.g., release-1.8)
- Updates `images_digest_mirror_set.yaml` with new image tags
- Renames tekton pipelines: globalhub-1-7 → globalhub-1-8 (pull-request and push)
- Updates bundle image labels and `konflux-patch.sh` image references
- Creates PR to the release branch
- Updates `main` branch with latest bundle content from multicluster-global-hub main (manifests, metadata, tests) and renamed tekton pipelines
- Creates cleanup PR to remove old GitHub Actions from previous release branch

**Expected PRs**: 3

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to release branch | `release-1.8` | tekton pipelines renamed globalhub-1-7 → globalhub-1-8; image tags updated to globalhub-1-8 | Check `.tekton/` has `*globalhub-1-8*.yaml` files |
| PR to main | `main` | Bundle content synced from MGH main (manifests, metadata, tests); tekton pipelines renamed globalhub-1-7 → globalhub-1-8 (target_branch=main) | Check `.tekton/` has `*globalhub-1-8*.yaml` with `target_branch=main`; bundle/ matches MGH |
| Cleanup PR | `release-1.7` | Remove `.github/workflows/` GitHub Actions from old release branch | Check old branch no longer has Actions workflows |

---

### Repository 4: operator-catalog (Script 04)

**Repository**: [stolostron/multicluster-global-hub-operator-catalog](https://github.com/stolostron/multicluster-global-hub-operator-catalog)

**What it does**:
- Creates/updates catalog branch (e.g., release-1.8)
- Updates `images-mirror-set.yaml` with new image tags
- **Adds** new OCP version pipeline (e.g., OCP 4.22 for release-1.8)
- **Removes** oldest OCP version pipeline (e.g., OCP 4.17)
- Updates existing OCP version pipelines (4.18-4.21)
- Updates `README.md` with new version information
- Updates GitHub Actions workflow for new release branch
- Creates PR to main with new release configuration
- Creates PR to catalog release branch with pipeline updates
- Creates cleanup PR to remove GitHub Actions from old release branch

**OCP Version Formula**: OCP_MIN = 4.(10 + GH_MINOR), OCP_MAX = OCP_MIN + 4

**Expected PRs**: 3

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to main | `main` | New release-1.8 catalog config; OCP version lifecycle managed | Check README lists OCP 4.18-4.22; new OCP pipeline added |
| PR to catalog branch | `release-1.8` | pipeline updates, image mirror set updated | Check `images-mirror-set.yaml` has globalhub-1-8 tags |
| Cleanup PR | `release-1.7` | Remove `.github/workflows/` from old release branch | Check old branch no longer has Actions workflows |

---

### Repository 5: glo-grafana (Script 05)

**Repository**: [stolostron/glo-grafana](https://github.com/stolostron/glo-grafana)

**What it does**:
- Checks out grafana release branch (e.g., release-1.8)
- Renames tekton pipelines: globalhub-1-7 → globalhub-1-8 (pull-request and push)
- Updates branch references in pipeline files
- Creates PR to the release branch

**Expected PRs**: 1

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to grafana branch | `release-1.8` | tekton pipelines renamed globalhub-1-7 → globalhub-1-8; branch references updated | Check `.tekton/` has `glo-grafana-globalhub-1-8-pull-request.yaml` and `glo-grafana-globalhub-1-8-push.yaml` |

---

### Repository 6: postgres_exporter (Script 06)

**Repository**: [stolostron/postgres_exporter](https://github.com/stolostron/postgres_exporter)

**What it does**:
- Checks out postgres_exporter release branch (e.g., release-2.17)
- Renames tekton pipelines: globalhub-1-7 → globalhub-1-8 (pull-request and push)
- Updates branch references in pipeline files
- Creates PR to the release branch

**Expected PRs**: 1

| PR | Target Branch | Content | Verification |
|----|--------------|---------|--------------|
| PR to release branch | `release-2.17` | tekton pipelines renamed globalhub-1-7 → globalhub-1-8; branch references updated | Check `.tekton/` has `postgres-exporter-globalhub-1-8-pull-request.yaml` and `postgres-exporter-globalhub-1-8-push.yaml` |

---

## Total Expected Output Summary

| Repo | PRs | Branches |
|------|-----|---------|
| multicluster-global-hub | 2 (main, prev-release) | release-2.17 |
| openshift/release | 1 (main) | — |
| operator-bundle | 3 (release-1.8, main, cleanup release-1.7) | release-1.8 |
| operator-catalog | 3 (main, release-1.8, cleanup release-1.7) | release-1.8 |
| glo-grafana | 1 (release-1.8) | release-1.8 |
| postgres_exporter | 1 (release-2.17) | release-2.17 |
| **Total** | **11 PRs** | **5 branches** |

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

**IMPORTANT**: RELEASE_BRANCH environment variable is REQUIRED for all execution modes.

The main orchestration script supports three modes:

### 1. Interactive Mode (Default)
```bash
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh
```
Prompts user to select which repositories to update (1-6, comma-separated, or all).

### 2. All Repositories Mode
```bash
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all
```
Updates all 6 repositories in sequence.

### 3. Selective Mode
```bash
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh 1,2,3    # Update specific repos
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh 3,4      # Update only bundle and catalog
```

### 4. CREATE_BRANCHES Mode (Create New Branches)
```bash
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all
```
Creates new release branches and pushes directly to upstream (requires write access).

### 5. UPDATE Mode (Default - Create PRs)
```bash
CREATE_BRANCHES=false RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all
# or simply:
RELEASE_BRANCH=release-2.17 ./scripts/cut-release.sh all
```
Updates existing release branches by creating pull requests.

### 6. Standalone Script Execution
Each script can be run independently:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" CREATE_BRANCHES=false ./scripts/03-bundle.sh
```

## Environment Variables

The main orchestration script calculates and exports these variables for child scripts:

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
| `OCP_MIN` | Minimum OCP version number | `418` | Auto-calculated |
| `OCP_MAX` | Maximum OCP version number | `422` | Auto-calculated |
| `OPENSHIFT_RELEASE_PATH` | Path to openshift/release clone | `/tmp/openshift-release` | Optional |
| `WORK_DIR` | Working directory for repos | `/tmp/globalhub-release-repos` | Optional |

Individual scripts can be run standalone by providing the required variables.

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

**Pull Requests Expected**: up to 11 PRs

> Each PR is only created if changes are needed. If the target branch is already up-to-date or an open PR already exists, the script will skip or reuse it.

| # | Repo | Target Branch | Content |
|---|------|--------------|---------|
| 1 | multicluster-global-hub | `main` | New globalhub-1-8 tekton configs, Containerfile/Makefile version bump |
| 2 | multicluster-global-hub | `release-2.16` | Update globalhub-1-7 pull-request pipeline target_branch → release-2.16 |
| 3 | openshift/release | `main` | CI config for release-2.17, presubmit/postsubmit jobs |
| 4 | operator-bundle | `release-1.8` | Rename tekton pipelines globalhub-1-7 → globalhub-1-8, update image tags |
| 5 | operator-bundle | `main` | Sync bundle content from MGH main; rename tekton pipelines globalhub-1-7 → globalhub-1-8 (target_branch=main) |
| 6 | operator-bundle | `release-1.7` | Cleanup old GitHub Actions workflows |
| 7 | operator-catalog | `main` | New release-1.8 catalog config, OCP 4.18-4.22 lifecycle |
| 8 | operator-catalog | `release-1.8` | Update pipeline and image mirror set |
| 9 | operator-catalog | `release-1.7` | Cleanup old GitHub Actions workflows |
| 10 | glo-grafana | `release-1.8` | Rename tekton pipelines globalhub-1-7 → globalhub-1-8 |
| 11 | postgres_exporter | `release-2.17` | Rename tekton pipelines globalhub-1-7 → globalhub-1-8 |

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

### Update all repositories (UPDATE mode - creates PRs):
```bash
RELEASE_BRANCH=release-2.17 ./.claude/skills/new-release/scripts/cut-release.sh all
```

### Create new release branches (CREATE_BRANCHES mode - pushes directly):
```bash
CREATE_BRANCHES=true RELEASE_BRANCH=release-2.18 ./.claude/skills/new-release/scripts/cut-release.sh all
```

### Interactive selection:
```bash
RELEASE_BRANCH=release-2.17 ./.claude/skills/new-release/scripts/cut-release.sh
# Then select: 3,4 (to update only bundle and catalog)
```

### Update specific repositories:
```bash
RELEASE_BRANCH=release-2.17 ./.claude/skills/new-release/scripts/cut-release.sh 1,2,3
```

### Standalone script execution:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" CREATE_BRANCHES=false ./scripts/04-catalog.sh
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
   Supported OCP:        4.18 - 4.22
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
