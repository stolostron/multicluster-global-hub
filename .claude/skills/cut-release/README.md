# Global Hub Release Workflow (Cut Release)

Automates the complete end-to-end workflow for cutting a new Multicluster Global Hub release across all repositories.

## Quick Start

### Using Claude Code

Simply ask Claude:

```
Cut a new Global Hub release
```

or

```
Cut Global Hub release for ACM 2.17
```

Claude will automatically invoke this skill and guide you through the process.

### Using Scripts Directly

```bash
# Interactive mode - choose which repos to update
./scripts/cut-release.sh

# Update all repositories
./scripts/cut-release.sh all

# Update specific repositories
./scripts/cut-release.sh 1,2    # Only main repo and openshift/release
./scripts/cut-release.sh 3,4,5,6  # Only bundle, catalog, grafana, postgres

# Specify release version
RELEASE_BRANCH="release-2.17" ./scripts/cut-release.sh all
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
- Creates release branch (e.g., release-2.17)
- Updates `.tekton/` pipelinesascode configurations
- Updates `Containerfile.*` version references
- **Creates PR** to bump version in main branch for next development cycle

**Example**:
```bash
RELEASE_BRANCH="release-2.17" ./scripts/01-multicluster-global-hub.sh
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
RELEASE_BRANCH="release-2.17" ./scripts/02-openshift-release.sh
```

### 3. Operator Bundle
**Script**: `03-bundle.sh`

**What it does**:
- Creates bundle branch (e.g., release-1.7)
- Updates `images_digest_mirror_set.yaml`
- Renames and updates tekton pipelines
- Updates bundle image labels
- Updates `konflux-patch.sh`
- **Creates PR** with all bundle changes

**Example**:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/03-bundle.sh
```

### 4. Operator Catalog
**Script**: `04-catalog.sh`

**What it does**:
- Creates catalog branch (e.g., release-1.7)
- Updates `images-mirror-set.yaml`
- **Adds** new OCP version pipelines (e.g., OCP 4.21)
- **Removes** old OCP version pipelines (e.g., OCP 4.16)
- Updates existing OCP pipelines (4.17-4.20)
- Updates README.md
- Updates GitHub Actions workflow
- **Creates 2 PRs**:
  - Main PR: New release configuration
  - Cleanup PR: Remove GitHub Actions from old branch

**Example**:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/04-catalog.sh
```

### 5. Glo-Grafana
**Script**: `05-grafana.sh`

**What it does**:
- Creates grafana branch (e.g., release-1.7)
- Renames and updates tekton pipelines

**Example**:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/05-grafana.sh
```

### 6. Postgres Exporter
**Script**: `06-postgres-exporter.sh`

**What it does**:
- Creates postgres_exporter branch (same as ACM, e.g., release-2.17)
- Renames and updates tekton pipelines

**Example**:
```bash
RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" ./scripts/06-postgres-exporter.sh
```

## Environment Variables

The main orchestration script (`cut-release.sh`) calculates and exports these variables:

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
$ ./scripts/cut-release.sh all

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

### Example 2: Update Only Specific Repositories

```bash
$ ./scripts/cut-release.sh 1,2

Mode: Update selected repositories: 1 2

... (updates only multicluster-global-hub and openshift/release)
```

### Example 3: Interactive Mode

```bash
$ ./scripts/cut-release.sh

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

### Example 4: Standalone Script Execution

```bash
# Run only the bundle script
$ RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" \
  ./scripts/03-bundle.sh

# Run only the catalog script
$ RELEASE_BRANCH="release-2.17" GH_VERSION="v1.8.0" \
  ./scripts/04-catalog.sh
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
