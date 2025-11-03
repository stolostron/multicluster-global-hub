#!/bin/bash

set -euo pipefail

# OpenShift Release Configuration Script
# Updates OpenShift CI configurations for new Global Hub release
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH           - Release branch name (e.g., release-2.17)
#   ACM_VERSION              - ACM version (e.g., 2.17)
#   GH_VERSION               - Global Hub version (e.g., v1.8.0)
#   OPENSHIFT_RELEASE_PATH   - Path to openshift/release clone

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$ACM_VERSION" ] || [ -z "$GH_VERSION" ] || [ -z "$OPENSHIFT_RELEASE_PATH" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, ACM_VERSION, GH_VERSION, OPENSHIFT_RELEASE_PATH"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS requires -i with empty string
  SED_INPLACE=(-i "")
else
  # Linux uses -i without argument
  SED_INPLACE=(-i)
fi

echo "🚀 OpenShift Release Configuration for $RELEASE_BRANCH"
echo "================================================"
echo "   ACM Version: $ACM_VERSION"
echo "   Global Hub Version: $GH_VERSION"
echo ""

# Step 1: Setup OpenShift release repository
echo ""
echo "📍 Step 1: Setting up OpenShift release repository..."

# Auto-detect GitHub user
GITHUB_USER=$(git remote -v | grep origin | head -1 | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
echo "   GitHub user: $GITHUB_USER"

# Check if already forked
if gh repo view "$GITHUB_USER/release" --json name >/dev/null 2>&1; then
  echo "   ✅ Fork already exists, skipping fork step"
else
  echo "   ⚠️  Fork not found. Please fork https://github.com/openshift/release to your account first."
  exit 1
fi

# Clone or use existing
if [ -d "$OPENSHIFT_RELEASE_PATH" ]; then
  echo "   ✅ Using existing clone at $OPENSHIFT_RELEASE_PATH"
  cd "$OPENSHIFT_RELEASE_PATH"

  # Ensure upstream remote exists
  git remote add upstream https://github.com/openshift/release.git 2>/dev/null || true

  # Fetch and update to latest
  echo "   Updating to latest upstream/master..."
  if git fetch upstream master; then
    git checkout master 2>/dev/null || git checkout main 2>/dev/null || true
    git pull upstream master 2>/dev/null || git pull upstream main 2>/dev/null || true
    echo "   ✅ Updated to latest commit"
  else
    echo "   ⚠️  Failed to update, continuing with existing state"
  fi
else
  echo "   📥 Cloning to $OPENSHIFT_RELEASE_PATH (--depth=1 for faster clone)..."
  PARENT_DIR=$(dirname "$OPENSHIFT_RELEASE_PATH")
  REPO_NAME=$(basename "$OPENSHIFT_RELEASE_PATH")
  cd "$PARENT_DIR"
  git clone --depth=1 --single-branch --branch master --progress "https://github.com/$GITHUB_USER/release.git" "$REPO_NAME" 2>&1 | grep -E "Receiving|Resolving" || true
  cd "$REPO_NAME"
  echo "   Adding upstream remote..."
  git remote add upstream https://github.com/openshift/release.git
  echo "   Fetching upstream master..."
  git fetch --depth=1 upstream master --progress 2>&1 | grep -E "Receiving|Resolving" || true
fi

# Detect latest release from multicluster-global-hub configs
echo "   🔍 Detecting previous release from existing configs..."
LATEST_RELEASE=$(find ci-operator/config/stolostron/multicluster-global-hub/ -name 'stolostron-multicluster-global-hub-release-*.yaml' 2>/dev/null | \
  sed 's|.*/stolostron-multicluster-global-hub-||' | \
  sed 's|\.yaml||' | \
  sort -V | tail -1)

if [ -z "$LATEST_RELEASE" ]; then
  echo "   ❌ Error: Could not detect previous release from openshift/release configs"
  exit 1
fi

echo "   Previous release detected: $LATEST_RELEASE"

# Calculate version strings for file updates
PREV_VERSION=$(echo "$LATEST_RELEASE" | sed 's/release-//')
VERSION_SHORT=$(echo "$ACM_VERSION" | tr -d '.')
PREV_VERSION_SHORT=$(echo "$PREV_VERSION" | tr -d '.')

echo "   Previous ACM Version: $PREV_VERSION"
echo "   Job Prefix: release-${VERSION_SHORT}"

# Create working branch
BRANCH_NAME="${RELEASE_BRANCH}-config"
if git show-ref --verify --quiet "refs/heads/$BRANCH_NAME"; then
  git checkout "$BRANCH_NAME"
  echo "   ✅ Switched to existing branch $BRANCH_NAME"
else
  git checkout -b "$BRANCH_NAME" upstream/master
  echo "   ✅ Created branch $BRANCH_NAME"
fi

# Step 2: Update CI configurations
echo ""
echo "📍 Step 2: Updating CI configurations..."

MAIN_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml"
LATEST_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${LATEST_RELEASE}.yaml"
NEW_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}.yaml"

# Update main branch configuration
echo "   Updating main branch configuration..."
sed "${SED_INPLACE[@]}" "s/name: \"${PREV_VERSION}\"/name: \"${ACM_VERSION}\"/" "$MAIN_CONFIG"
sed "${SED_INPLACE[@]}" "s/DESTINATION_BRANCH: ${LATEST_RELEASE}/DESTINATION_BRANCH: ${RELEASE_BRANCH}/" "$MAIN_CONFIG"
echo "   ✅ Updated $MAIN_CONFIG"

# Create new release configuration
echo "   Creating $RELEASE_BRANCH pipeline configuration..."

# Idempotent: check if file exists and is already updated
if [ -f "$NEW_CONFIG" ]; then
  echo "   ℹ️  Configuration file already exists: $NEW_CONFIG"
  # Check if it's already been updated with the correct version
  if grep -q "branch: ${RELEASE_BRANCH}" "$NEW_CONFIG" && \
     grep -q "IMAGE_TAG: ${GH_VERSION}" "$NEW_CONFIG"; then
    echo "   ✓ Configuration already up to date"
  else
    echo "   ⚠️  File exists but needs updates, applying changes..."
    # Update version references
    sed "${SED_INPLACE[@]}" "s/name: \"${PREV_VERSION}\"/name: \"${ACM_VERSION}\"/" "$NEW_CONFIG"
    sed "${SED_INPLACE[@]}" "s/branch: ${LATEST_RELEASE}/branch: ${RELEASE_BRANCH}/" "$NEW_CONFIG"
    sed "${SED_INPLACE[@]}" "s/release-${PREV_VERSION_SHORT}/release-${VERSION_SHORT}/g" "$NEW_CONFIG"
    sed "${SED_INPLACE[@]}" "s/IMAGE_TAG: v1\.[0-9]\+\.0/IMAGE_TAG: ${GH_VERSION}/" "$NEW_CONFIG"
    echo "   ✅ Updated $NEW_CONFIG"
  fi
else
  cp "$LATEST_CONFIG" "$NEW_CONFIG"
  # Update version references in new config
  sed "${SED_INPLACE[@]}" "s/name: \"${PREV_VERSION}\"/name: \"${ACM_VERSION}\"/" "$NEW_CONFIG"
  sed "${SED_INPLACE[@]}" "s/branch: ${LATEST_RELEASE}/branch: ${RELEASE_BRANCH}/" "$NEW_CONFIG"
  sed "${SED_INPLACE[@]}" "s/release-${PREV_VERSION_SHORT}/release-${VERSION_SHORT}/g" "$NEW_CONFIG"
  sed "${SED_INPLACE[@]}" "s/IMAGE_TAG: v1\.[0-9]\+\.0/IMAGE_TAG: ${GH_VERSION}/" "$NEW_CONFIG"
  echo "   ✅ Created $NEW_CONFIG"
fi

# Step 3: Verify container engine and auto-generate job configurations
echo ""
echo "📍 Step 3: Verifying container engine availability..."

# Check for Docker
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  CONTAINER_ENGINE="docker"
  echo "   ✅ Docker is available and running"
# Check for Podman
elif command -v podman >/dev/null 2>&1; then
  # Check if podman machine is running
  if podman machine list 2>/dev/null | grep -q "Currently running"; then
    CONTAINER_ENGINE="podman"
    echo "   ✅ Podman is available and running"
  else
    echo "   ❌ Error: Podman is installed but no machine is running"
    echo ""
    echo "   Please start your podman machine:"
    echo "      podman machine start"
    echo ""
    exit 1
  fi
else
  echo "   ❌ Error: No container engine found!"
  echo ""
  echo "   Please ensure Docker or Podman is installed and running."
  echo "   - Docker: Start Docker Desktop application"
  echo "   - Podman: Ensure podman machine is running (podman machine start)"
  echo ""
  exit 1
fi

echo ""
echo "📍 Step 4: Auto-generating job configurations..."
echo "   Running make update (timeout: 2 minutes)..."
echo "   Using $CONTAINER_ENGINE as container engine..."

# Track if make update succeeded
MAKE_UPDATE_SUCCESS=false

# Run make update with 2 minute timeout
if [ "$CONTAINER_ENGINE" = "docker" ]; then
  if timeout 120 bash -c "CONTAINER_ENGINE=docker make update" 2>/dev/null; then
    MAKE_UPDATE_SUCCESS=true
    echo "   ✅ Job configurations generated"
  else
    echo "   ⚠️  make update timed out or failed (skipping)"
    echo "   ℹ️  You may need to run 'make update' manually in the PR"
  fi
else
  if timeout 120 make update 2>/dev/null; then
    MAKE_UPDATE_SUCCESS=true
    echo "   ✅ Job configurations generated"
  else
    echo "   ⚠️  make update timed out or failed (skipping)"
    echo "   ℹ️  You may need to run 'make update' manually in the PR"
  fi
fi

# Step 4: Commit and create PR
echo ""
echo "📍 Step 5: Committing changes and creating PR..."

# Check if there are changes
if git diff --quiet && git diff --cached --quiet; then
  echo "   ⚠️  No changes to commit"
  exit 0
fi

# Add files
git add "$MAIN_CONFIG"
git add "$NEW_CONFIG"

# Add job files if they exist (may not exist if make update failed)
if [ "$MAKE_UPDATE_SUCCESS" = true ]; then
  git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}-presubmits.yaml" 2>/dev/null || true
  git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}-postsubmits.yaml" 2>/dev/null || true
  COMMIT_NOTE="- Auto-generate presubmits and postsubmits using make update"
else
  COMMIT_NOTE="- Job files need to be generated manually (make update failed/timed out)"
fi

# Commit
git commit -m "Add ${RELEASE_BRANCH} configuration for multicluster-global-hub

- Update main branch to promote to ${ACM_VERSION} and fast-forward to ${RELEASE_BRANCH}
- Create ${RELEASE_BRANCH} pipeline configuration based on ${LATEST_RELEASE}
- Update image-mirror job prefixes to release-${VERSION_SHORT}
- IMAGE_TAG is ${GH_VERSION} (corresponding to ACM ${ACM_VERSION} / Global Hub ${GH_VERSION})
${COMMIT_NOTE}"

echo "   ✅ Changes committed"

# Push
git push -u origin "$BRANCH_NAME"
echo "   ✅ Pushed to fork"

# Create PR
PR_URL=$(gh pr create --base master --head "$GITHUB_USER:$BRANCH_NAME" \
  --title "Add ${RELEASE_BRANCH} configuration for multicluster-global-hub" \
  --body "This PR adds ${RELEASE_BRANCH} configuration for the multicluster-global-hub project.

## Changes

1. **Update main branch configuration**: Promote to ${ACM_VERSION}, fast-forward to ${RELEASE_BRANCH}
2. **Create ${RELEASE_BRANCH} pipeline configuration**: Based on ${LATEST_RELEASE}
3. **Auto-generate job configurations**: Using \`make update\`

## Version Mapping

- Release branch: \`${RELEASE_BRANCH}\`
- ACM version: ${ACM_VERSION}
- Global Hub version: ${GH_VERSION}
- Job prefix: \`release-${VERSION_SHORT}\`" \
  --repo openshift/release)

echo "   ✅ Created PR: $PR_URL"

# Summary
echo ""
echo "================================================"
echo "📊 SCRIPT SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Updated main branch config (promote to ${ACM_VERSION}, fast-forward to ${RELEASE_BRANCH})"
echo "  ✓ Created ${RELEASE_BRANCH} pipeline config (based on ${LATEST_RELEASE})"
if [ "$MAKE_UPDATE_SUCCESS" = true ]; then
  echo "  ✓ Auto-generated presubmit/postsubmit jobs"
else
  echo "  ⚠️  Auto-generate jobs skipped (timeout/error)"
fi
echo "  ✓ PR created"
echo ""
echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"
echo ""
echo "Review and merge PR:"
echo "  ${PR_URL}"
echo ""
if [ "$MAKE_UPDATE_SUCCESS" != true ]; then
  echo "⚠️  IMPORTANT - make update failed/timed out:"
  echo "  After PR created, you need to:"
  echo "  1. cd $OPENSHIFT_RELEASE_PATH"
  echo "  2. git checkout $BRANCH_NAME"
  echo "  3. make update"
  echo "  4. git add ci-operator/jobs/"
  echo "  5. git commit --amend --no-edit"
  echo "  6. git push -f"
  echo ""
fi
echo "After PR merged:"
echo "  - Verify CI jobs are created correctly"
echo "  - Check promotion target is set to ${VERSION}"
echo ""
echo "================================================"
if [ "$MAKE_UPDATE_SUCCESS" = true ]; then
  echo "✅ SUCCESS (4 tasks completed)"
else
  echo "⚠️  COMPLETED WITH WARNINGS (3/4 tasks completed)"
fi
echo "================================================"
