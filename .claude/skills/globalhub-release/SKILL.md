---
name: globalhub-release
description: Automate creation of new Multicluster Global Hub release branches and OpenShift CI configurations. Use when creating new release branches (e.g., release-2.16, release-2.17), updating OpenShift release repository configs, or setting up CI/CD for new releases. Keywords: release branch, global hub, openshift/release, prow, CI configuration.
allowed-tools: [Read, Write, Bash, Glob, Grep]
---

# Global Hub Release Branch Creation

Automates the complete workflow for creating a new Multicluster Global Hub release branch and corresponding OpenShift CI configurations.

## When to Use This Skill

- Creating a new Global Hub release branch (e.g., release-2.16, release-2.17)
- Setting up OpenShift CI configurations for a new release
- Updating prow configurations for new release branches
- Automating release branch workflows

## Workflow Overview

1. Detect latest release branch and determine next release version
2. Create new release branch in Global Hub repository
3. Fork and setup OpenShift release repository
4. Update CI configurations (main branch, new release configs)
5. Auto-generate job configurations (presubmits, postsubmits)
6. Create pull request to openshift/release

## Configuration Variables

- `RELEASE_NAME`: Release version (default: "next" - auto-detect next version)
- `OPENSHIFT_RELEASE_PATH`: Local path for openshift/release repo (default: "/tmp/openshift-release")
- `GITHUB_USER`: Auto-detected from git config

## Platform Compatibility

This workflow is compatible with:
- **macOS** (ARM/Apple Silicon and Intel) - Uses `sed -i ""` syntax
- **Linux** (x86_64) - Uses `sed -i` syntax

The script automatically detects the OS and adjusts accordingly.

## Instructions

### Step 1: Detect Latest Release and Determine Next Version

```bash
# Fetch all branches
git fetch origin

# Find latest release branch
LATEST_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | sed 's|origin/||' | sort -V | tail -1)

# Determine next release
if [ "$RELEASE_NAME" = "next" ]; then
  # Extract version numbers (e.g., release-2.15 -> 2.15)
  MAJOR_MINOR=$(echo $LATEST_RELEASE | sed 's/release-//')
  MAJOR=$(echo $MAJOR_MINOR | cut -d. -f1)
  MINOR=$(echo $MAJOR_MINOR | cut -d. -f2)
  NEXT_MINOR=$((MINOR + 1))
  NEXT_RELEASE="release-${MAJOR}.${NEXT_MINOR}"
else
  NEXT_RELEASE="$RELEASE_NAME"
fi
```

### Step 2: Create New Release Branch in Global Hub Repository

```bash
# Create and push new release branch based on main
git fetch origin main:refs/remotes/origin/main
git branch $NEXT_RELEASE origin/main
git push origin $NEXT_RELEASE
```

### Step 3: Setup OpenShift Release Repository

**Auto-detect GitHub username:**
```bash
GITHUB_USER=$(git remote -v | grep origin | head -1 | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
```

**Check if already forked:**
```bash
gh repo view $GITHUB_USER/release --json name 2>&1 | grep -q "Could not resolve" && FORKED="false" || FORKED="true"
```

**Clone or use existing:**
```bash
if [ -z "$OPENSHIFT_RELEASE_PATH" ]; then
  OPENSHIFT_RELEASE_PATH="/tmp/openshift-release"
fi

if [ ! -d "$OPENSHIFT_RELEASE_PATH" ]; then
  cd $(dirname $OPENSHIFT_RELEASE_PATH)
  git clone --depth 1 https://github.com/$GITHUB_USER/release.git $(basename $OPENSHIFT_RELEASE_PATH)
fi

cd $OPENSHIFT_RELEASE_PATH
git remote add upstream https://github.com/openshift/release.git 2>/dev/null || true
git fetch --depth 1 upstream master
git checkout -b ${NEXT_RELEASE}-config upstream/master
```

### Step 4: Update CI Configurations

**Extract version numbers for naming conventions:**
```bash
# Example: release-2.16 -> 216 (for job names)
VERSION_SHORT=$(echo $NEXT_RELEASE | sed 's/release-//' | tr -d '.')
PREV_VERSION_SHORT=$(echo $LATEST_RELEASE | sed 's/release-//' | tr -d '.')

# Example: release-2.16 -> 2.16 (for version references)
VERSION=$(echo $NEXT_RELEASE | sed 's/release-//')
PREV_VERSION=$(echo $LATEST_RELEASE | sed 's/release-//')

# Calculate Global Hub version (ACM version mapping)
# ACM 2.16 -> Global Hub v1.7.0 (pattern: (minor - 9) / 10 + 1.0)
MINOR=$(echo $VERSION | cut -d. -f2)
GH_MINOR=$((MINOR - 9))
GH_VERSION="v1.${GH_MINOR}.0"
```

**Update main branch configuration:**
```bash
# File: ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml

# Update promotion target
sed -i "s/name: \"${PREV_VERSION}\"/name: \"${VERSION}\"/" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml

# Update fast-forward branch
sed -i "s/DESTINATION_BRANCH: ${LATEST_RELEASE}/DESTINATION_BRANCH: ${NEXT_RELEASE}/" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml
```

**Create new release pipeline configuration:**
```bash
# Copy from latest release
cp ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${LATEST_RELEASE}.yaml \
   ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml

# Update version references
sed -i "s/name: \"${PREV_VERSION}\"/name: \"${VERSION}\"/" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml

# Update branch metadata
sed -i "s/branch: ${LATEST_RELEASE}/branch: ${NEXT_RELEASE}/" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml

# Update image-mirror job names (e.g., release-214 -> release-216)
sed -i "s/release-${PREV_VERSION_SHORT}/release-${VERSION_SHORT}/g" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml

# Update IMAGE_TAG to new version
sed -i "s/IMAGE_TAG: v1\.[0-9]\+\.0/IMAGE_TAG: ${GH_VERSION}/" \
  ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml
```

### Step 5: Verify Container Engine and Auto-Generate Job Configurations

**First, verify that a container engine is available:**

```bash
# Check for Docker
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  CONTAINER_ENGINE="docker"
  echo "✅ Docker is available and running"
# Check for Podman
elif command -v podman >/dev/null 2>&1; then
  if podman machine list 2>/dev/null | grep -q "Currently running"; then
    CONTAINER_ENGINE="podman"
    echo "✅ Podman is available and running"
  else
    echo "❌ Error: Podman is installed but no machine is running"
    echo "Please start your podman machine: podman machine start"
    exit 1
  fi
else
  echo "❌ Error: No container engine found!"
  echo "Please ensure Docker or Podman is installed and running"
  exit 1
fi
```

**Then use `make update` to automatically generate presubmits and postsubmits:**

```bash
if [ "$CONTAINER_ENGINE" = "docker" ]; then
  CONTAINER_ENGINE=docker make update
else
  make update
fi
```

This will automatically generate:
- `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-presubmits.yaml`
- `ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-postsubmits.yaml`

### Step 6: Commit and Create Pull Request

```bash
# Add all changes
git add ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml
git add ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml
git add ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-presubmits.yaml
git add ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-postsubmits.yaml

# Commit changes
git commit -m "Add ${NEXT_RELEASE} configuration for multicluster-global-hub

- Update main branch to promote to ${VERSION} and fast-forward to ${NEXT_RELEASE}
- Create ${NEXT_RELEASE} pipeline configuration based on ${LATEST_RELEASE}
- Update image-mirror job prefixes to release-${VERSION_SHORT}
- IMAGE_TAG is ${GH_VERSION} (corresponding to ACM ${VERSION} / Global Hub ${GH_VERSION})
- Auto-generate presubmits and postsubmits using make update"

# Push to fork
git push -u origin ${NEXT_RELEASE}-config

# Create PR
gh pr create --base master --head $GITHUB_USER:${NEXT_RELEASE}-config \
  --title "Add ${NEXT_RELEASE} configuration for multicluster-global-hub" \
  --body "This PR adds ${NEXT_RELEASE} configuration for the multicluster-global-hub project.

## Changes

1. **Update main branch configuration**: Promote to ${VERSION}, fast-forward to ${NEXT_RELEASE}
2. **Create ${NEXT_RELEASE} pipeline configuration**: Based on ${LATEST_RELEASE}
3. **Auto-generate job configurations**: Using \`make update\`

## Version Mapping

- Release branch: \`${NEXT_RELEASE}\`
- ACM version: ${VERSION}
- Global Hub version: ${GH_VERSION}
- Job prefix: \`release-${VERSION_SHORT}\`" \
  --repo openshift/release
```

## Version Mapping Rules

Global Hub versions follow this pattern relative to ACM versions:

| ACM Version | Release Branch | Global Hub Version | Job Prefix |
|-------------|----------------|-------------------|------------|
| 2.14        | release-2.14   | v1.5.0            | release-214 |
| 2.15        | release-2.15   | v1.6.0            | release-215 |
| 2.16        | release-2.16   | v1.7.0            | release-216 |
| 2.17        | release-2.17   | v1.8.0            | release-217 |

Formula: `Global Hub v1.X.0` where `X = ACM_MINOR - 9`

## File Naming Conventions

- **Branch names**: `release-X.Y` (e.g., `release-2.16`)
- **Config files**: `stolostron-multicluster-global-hub-release-X.Y.yaml`
- **Job prefixes**: `release-XY` (no dot, e.g., `release-216`)
- **Image tags**: `vX.Y.Z` (e.g., `v1.7.0`)

## Best Practices

1. **Always verify latest release first**: Don't assume - check with `git fetch && git branch -r`
2. **Use shallow clones**: Save time and bandwidth with `--depth 1`
3. **Auto-detect GitHub user**: Never hardcode usernames
4. **Let make update generate jobs**: Don't create job files manually
5. **Check fork status**: Don't try to fork if already forked
6. **Reuse existing clones**: Check if directory exists before cloning

## Error Handling

- **If docker/podman fails**: Ensure container engine is running before `make update`
- **If fork check fails**: Verify `gh` CLI is authenticated
- **If push fails**: Check if remote branch already exists
- **If make update fails**: Check YAML syntax in config files

## Common Issues

1. **Container engine not running**: Start docker/podman before running `make update`
2. **Outdated openshift/release clone**: Always `git fetch upstream master` before creating new branch
3. **Wrong version calculations**: Double-check the version mapping formula
4. **Job naming mismatch**: Ensure job prefixes match (no dot): `release-216` not `release-2.16`

## Example Usage

**Auto-detect next version:**
```bash
RELEASE_NAME="next" ./create-release.sh
```

**Specify version explicitly:**
```bash
RELEASE_NAME="release-2.17" ./create-release.sh
```

**Custom openshift/release path:**
```bash
OPENSHIFT_RELEASE_PATH="$HOME/workspace/openshift-release" RELEASE_NAME="next" ./create-release.sh
```

## Output Format

The skill should provide clear progress indicators:

```
✅ Detected latest release: release-2.15
✅ Next release will be: release-2.16
✅ Created release-2.16 branch in Global Hub repository
✅ GitHub user detected: yanmxa
✅ Fork already exists, skipping fork step
✅ Cloned openshift/release to /tmp/openshift-release
✅ Updated main branch configuration
✅ Created release-2.16 pipeline configuration
✅ Running make update to generate jobs...
✅ Job configurations generated successfully
✅ Committed changes
✅ Created PR: https://github.com/openshift/release/pull/70887

📋 Summary:
   - Release: release-2.16
   - ACM Version: 2.16
   - Global Hub Version: v1.7.0
   - PR: https://github.com/openshift/release/pull/70887
```
