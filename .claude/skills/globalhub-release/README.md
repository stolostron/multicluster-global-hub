# Global Hub Release Branch Creation Skill

This skill automates the complete workflow for creating a new Multicluster Global Hub release branch and corresponding OpenShift CI configurations.

## Quick Start

### Using Claude

Simply ask Claude to create a new release:

```
Create a new Global Hub release branch
```

or

```
Create Global Hub release-2.17
```

Claude will automatically invoke this skill and guide you through the process.

### Using the Script Directly

```bash
# Auto-detect next version
./.claude/skills/globalhub-release/scripts/create-release.sh

# Or with environment variables
RELEASE_NAME="next" ./.claude/skills/globalhub-release/scripts/create-release.sh

# Specify version explicitly
RELEASE_NAME="release-2.17" ./.claude/skills/globalhub-release/scripts/create-release.sh

# Custom openshift/release path
OPENSHIFT_RELEASE_PATH="$HOME/workspace/openshift-release" RELEASE_NAME="next" ./.claude/skills/globalhub-release/scripts/create-release.sh
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `RELEASE_NAME` | Release version (use "next" for auto-detect) | `next` |
| `OPENSHIFT_RELEASE_PATH` | Local path for openshift/release repo | `/tmp/openshift-release` |

## Prerequisites

1. **Git configured**: With your GitHub credentials
2. **GitHub CLI (`gh`)**: Authenticated with your account
3. **Fork**: You must have forked https://github.com/openshift/release to your account
4. **Container engine**: Either Docker or Podman running (for `make update`)

## What This Skill Does

1. ✅ Detects the latest release branch (e.g., release-2.15)
2. ✅ Calculates the next release version (e.g., release-2.16)
3. ✅ Creates new release branch in Global Hub repository
4. ✅ Auto-detects your GitHub username
5. ✅ Checks if openshift/release is already forked
6. ✅ Clones or reuses existing openshift/release repository
7. ✅ Updates main branch CI configuration
8. ✅ Creates new release pipeline configuration
9. ✅ Auto-generates presubmits and postsubmits jobs
10. ✅ Commits all changes with proper commit message
11. ✅ Creates pull request to openshift/release

## Version Mapping

The skill automatically calculates correct versions:

| ACM Version | Release Branch | Global Hub Version | Job Prefix |
|-------------|----------------|-------------------|------------|
| 2.15        | release-2.15   | v1.6.0            | release-215 |
| 2.16        | release-2.16   | v1.7.0            | release-216 |
| 2.17        | release-2.17   | v1.8.0            | release-217 |

**Formula**: Global Hub version = `v1.X.0` where `X = ACM_MINOR - 9`

## Files Modified/Created

### In Global Hub Repository
- Creates new branch: `release-X.Y`

### In openshift/release Repository
1. **Modified**: `ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml`
   - Updates promotion target
   - Updates fast-forward branch

2. **Created**: `ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-X.Y.yaml`
   - New release pipeline configuration
   - Correct image tags and job names

3. **Auto-generated**: `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-X.Y-presubmits.yaml`
   - Presubmit job configurations

4. **Auto-generated**: `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-release-X.Y-postsubmits.yaml`
   - Postsubmit job configurations

## Troubleshooting

### Container engine not running
**Error**: `Cannot connect to the Docker daemon` or `podman: command not found`

**Solution**: Start Docker Desktop or initialize podman:
```bash
# For Docker
# Start Docker Desktop application

# For Podman
podman machine init
podman machine start
```

### Fork not found
**Error**: `Fork not found`

**Solution**: Fork the repository first:
1. Go to https://github.com/openshift/release
2. Click "Fork" button
3. Run the script again

### GitHub CLI not authenticated
**Error**: `gh: command not found` or authentication errors

**Solution**: Install and authenticate gh CLI:
```bash
brew install gh
gh auth login
```

### make update fails
**Error**: YAML syntax errors or validation failures

**Solution**: Check the generated YAML files for syntax errors:
```bash
cd /tmp/openshift-release
# Fix syntax errors in config files
# Then run make update again
```

## Advanced Usage

### Using a specific openshift/release clone

If you maintain a local clone of openshift/release:

```bash
OPENSHIFT_RELEASE_PATH="$HOME/workspace/openshift-release" \
RELEASE_NAME="next" \
./.claude/skills/globalhub-release/scripts/create-release.sh
```

### Dry run (check what would be created)

The script doesn't have a dry-run mode yet, but you can manually inspect changes:

```bash
# Run the script
RELEASE_NAME="next" ./.claude/skills/globalhub-release/scripts/create-release.sh

# Before the PR is created, inspect changes
cd /tmp/openshift-release
git diff HEAD~1

# If you want to abort
git reset --hard HEAD~1
```

## Examples

### Example 1: Create next release (auto-detect)

```bash
$ ./.claude/skills/globalhub-release/scripts/create-release.sh

🚀 Starting Global Hub Release Branch Creation
================================================

📍 Step 1: Detecting latest release version...
   Latest release: release-2.15
   Next release: release-2.16
   ACM Version: 2.16
   Global Hub Version: v1.7.0
   Job Prefix: release-216

📍 Step 2: Creating new release branch in Global Hub repository...
   ✅ Created branch release-2.16
   ✅ Pushed branch release-2.16 to remote

📍 Step 3: Setting up OpenShift release repository...
   GitHub user: yanmxa
   ✅ Fork already exists, skipping fork step
   ✅ Using existing clone at /tmp/openshift-release
   ✅ Created branch release-2.16-config

📍 Step 4: Updating CI configurations...
   Updating main branch configuration...
   ✅ Updated ci-operator/config/.../main.yaml
   Creating release-2.16 pipeline configuration...
   ✅ Created ci-operator/config/.../release-2.16.yaml

📍 Step 5: Auto-generating job configurations...
   Running make update (this may take a few minutes)...
   Using Docker as container engine...
   ✅ Job configurations generated

📍 Step 6: Committing changes and creating PR...
   ✅ Changes committed
   ✅ Pushed to fork
   ✅ Created PR: https://github.com/openshift/release/pull/70888

================================================
✅ Release creation completed successfully!

📋 Summary:
   Release: release-2.16
   ACM Version: 2.16
   Global Hub Version: v1.7.0
   Job Prefix: release-216
   PR: https://github.com/openshift/release/pull/70888

🎉 Done!
```

### Example 2: Create specific release version

```bash
$ RELEASE_NAME="release-2.17" ./.claude/skills/globalhub-release/scripts/create-release.sh

🚀 Starting Global Hub Release Branch Creation
================================================

📍 Step 1: Detecting latest release version...
   Latest release: release-2.16
   Next release: release-2.17
   ACM Version: 2.17
   Global Hub Version: v1.8.0
   Job Prefix: release-217
...
```

## Contributing

To improve this skill:

1. Edit `.claude/skills/globalhub-release/SKILL.md` for instructions
2. Edit `.claude/skills/globalhub-release/scripts/create-release.sh` for automation
3. Test your changes with a dry run
4. Commit and share with the team

## License

This skill is part of the Multicluster Global Hub project.
