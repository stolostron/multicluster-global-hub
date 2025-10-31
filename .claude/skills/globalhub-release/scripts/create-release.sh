#!/bin/bash

set -euo pipefail

# Global Hub Release Branch Creation Script
#
# Usage:
#   RELEASE_NAME="next" ./create-release.sh
#   RELEASE_NAME="release-2.17" ./create-release.sh
#   OPENSHIFT_RELEASE_PATH="/custom/path" RELEASE_NAME="next" ./create-release.sh

# Configuration
RELEASE_NAME="${RELEASE_NAME:-next}"
OPENSHIFT_RELEASE_PATH="${OPENSHIFT_RELEASE_PATH:-/tmp/openshift-release}"

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS requires -i with empty string
  SED_INPLACE=(-i "")
else
  # Linux uses -i without argument
  SED_INPLACE=(-i)
fi

echo "🚀 Starting Global Hub Release Branch Creation"
echo "================================================"

# Step 1: Detect latest release and determine next version
echo ""
echo "📍 Step 1: Detecting latest release version..."
git fetch origin >/dev/null 2>&1

LATEST_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)
echo "   Latest release: $LATEST_RELEASE"

if [ "$RELEASE_NAME" = "next" ]; then
  MAJOR_MINOR=$(echo $LATEST_RELEASE | sed 's/release-//')
  MAJOR=$(echo $MAJOR_MINOR | cut -d. -f1)
  MINOR=$(echo $MAJOR_MINOR | cut -d. -f2)
  NEXT_MINOR=$((MINOR + 1))
  NEXT_RELEASE="release-${MAJOR}.${NEXT_MINOR}"
else
  NEXT_RELEASE="$RELEASE_NAME"
fi
echo "   Next release: $NEXT_RELEASE"

# Calculate versions
VERSION=$(echo $NEXT_RELEASE | sed 's/release-//')
PREV_VERSION=$(echo $LATEST_RELEASE | sed 's/release-//')
VERSION_SHORT=$(echo $VERSION | tr -d '.')
PREV_VERSION_SHORT=$(echo $PREV_VERSION | tr -d '.')

# Calculate Global Hub version
MINOR=$(echo $VERSION | cut -d. -f2)
GH_MINOR=$((MINOR - 9))
GH_VERSION="v1.${GH_MINOR}.0"

echo "   ACM Version: $VERSION"
echo "   Global Hub Version: $GH_VERSION"
echo "   Job Prefix: release-${VERSION_SHORT}"

# Step 2: Create new release branch
echo ""
echo "📍 Step 2: Creating new release branch in Global Hub repository..."
git fetch origin main:refs/remotes/origin/main >/dev/null 2>&1
if git show-ref --verify --quiet refs/heads/$NEXT_RELEASE; then
  echo "   ⚠️  Branch $NEXT_RELEASE already exists locally"
else
  git branch $NEXT_RELEASE origin/main
  echo "   ✅ Created branch $NEXT_RELEASE"
fi

if git ls-remote --heads origin $NEXT_RELEASE | grep -q $NEXT_RELEASE; then
  echo "   ⚠️  Branch $NEXT_RELEASE already exists on remote"
else
  git push origin $NEXT_RELEASE
  echo "   ✅ Pushed branch $NEXT_RELEASE to remote"
fi

# Step 3: Setup OpenShift release repository
echo ""
echo "📍 Step 3: Setting up OpenShift release repository..."

# Auto-detect GitHub user
GITHUB_USER=$(git remote -v | grep origin | head -1 | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
echo "   GitHub user: $GITHUB_USER"

# Check if already forked
if gh repo view $GITHUB_USER/release --json name >/dev/null 2>&1; then
  echo "   ✅ Fork already exists, skipping fork step"
else
  echo "   ⚠️  Fork not found. Please fork https://github.com/openshift/release to your account first."
  exit 1
fi

# Clone or use existing
if [ -d "$OPENSHIFT_RELEASE_PATH" ]; then
  echo "   ✅ Using existing clone at $OPENSHIFT_RELEASE_PATH"
  cd "$OPENSHIFT_RELEASE_PATH"
  git fetch upstream master >/dev/null 2>&1 || {
    git remote add upstream https://github.com/openshift/release.git
    git fetch upstream master >/dev/null 2>&1
  }
else
  echo "   📥 Cloning to $OPENSHIFT_RELEASE_PATH..."
  PARENT_DIR=$(dirname "$OPENSHIFT_RELEASE_PATH")
  REPO_NAME=$(basename "$OPENSHIFT_RELEASE_PATH")
  cd "$PARENT_DIR"
  git clone --depth 1 https://github.com/$GITHUB_USER/release.git "$REPO_NAME"
  cd "$REPO_NAME"
  git remote add upstream https://github.com/openshift/release.git
  git fetch --depth 1 upstream master
fi

# Create working branch
BRANCH_NAME="${NEXT_RELEASE}-config"
if git show-ref --verify --quiet refs/heads/$BRANCH_NAME; then
  git checkout $BRANCH_NAME
  echo "   ✅ Switched to existing branch $BRANCH_NAME"
else
  git checkout -b $BRANCH_NAME upstream/master
  echo "   ✅ Created branch $BRANCH_NAME"
fi

# Step 4: Update CI configurations
echo ""
echo "📍 Step 4: Updating CI configurations..."

MAIN_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-main.yaml"
LATEST_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${LATEST_RELEASE}.yaml"
NEW_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}.yaml"

# Update main branch configuration
echo "   Updating main branch configuration..."
sed "${SED_INPLACE[@]}" "s/name: \"${PREV_VERSION}\"/name: \"${VERSION}\"/" "$MAIN_CONFIG"
sed "${SED_INPLACE[@]}" "s/DESTINATION_BRANCH: ${LATEST_RELEASE}/DESTINATION_BRANCH: ${NEXT_RELEASE}/" "$MAIN_CONFIG"
echo "   ✅ Updated $MAIN_CONFIG"

# Create new release configuration
echo "   Creating $NEXT_RELEASE pipeline configuration..."
cp "$LATEST_CONFIG" "$NEW_CONFIG"

# Update version references in new config
sed "${SED_INPLACE[@]}" "s/name: \"${PREV_VERSION}\"/name: \"${VERSION}\"/" "$NEW_CONFIG"
sed "${SED_INPLACE[@]}" "s/branch: ${LATEST_RELEASE}/branch: ${NEXT_RELEASE}/" "$NEW_CONFIG"
sed "${SED_INPLACE[@]}" "s/release-${PREV_VERSION_SHORT}/release-${VERSION_SHORT}/g" "$NEW_CONFIG"
sed "${SED_INPLACE[@]}" "s/IMAGE_TAG: v1\.[0-9]\+\.0/IMAGE_TAG: ${GH_VERSION}/" "$NEW_CONFIG"
echo "   ✅ Created $NEW_CONFIG"

# Step 5: Verify container engine and auto-generate job configurations
echo ""
echo "📍 Step 5: Verifying container engine availability..."

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
echo "📍 Step 6: Auto-generating job configurations..."
echo "   Running make update (this may take a few minutes)..."
echo "   Using $CONTAINER_ENGINE as container engine..."

if [ "$CONTAINER_ENGINE" = "docker" ]; then
  CONTAINER_ENGINE=docker make update
else
  make update
fi

echo "   ✅ Job configurations generated"

# Step 7: Commit and create PR
echo ""
echo "📍 Step 7: Committing changes and creating PR..."

# Check if there are changes
if git diff --quiet && git diff --cached --quiet; then
  echo "   ⚠️  No changes to commit"
  exit 0
fi

# Add files
git add "$MAIN_CONFIG"
git add "$NEW_CONFIG"
git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-presubmits.yaml"
git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${NEXT_RELEASE}-postsubmits.yaml"

# Commit
git commit -m "Add ${NEXT_RELEASE} configuration for multicluster-global-hub

- Update main branch to promote to ${VERSION} and fast-forward to ${NEXT_RELEASE}
- Create ${NEXT_RELEASE} pipeline configuration based on ${LATEST_RELEASE}
- Update image-mirror job prefixes to release-${VERSION_SHORT}
- IMAGE_TAG is ${GH_VERSION} (corresponding to ACM ${VERSION} / Global Hub ${GH_VERSION})
- Auto-generate presubmits and postsubmits using make update"

echo "   ✅ Changes committed"

# Push
git push -u origin $BRANCH_NAME
echo "   ✅ Pushed to fork"

# Create PR
PR_URL=$(gh pr create --base master --head $GITHUB_USER:$BRANCH_NAME \
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
  --repo openshift/release)

echo "   ✅ Created PR: $PR_URL"

# Summary
echo ""
echo "================================================"
echo "✅ Release creation completed successfully!"
echo ""
echo "📋 Summary:"
echo "   Release: $NEXT_RELEASE"
echo "   ACM Version: $VERSION"
echo "   Global Hub Version: $GH_VERSION"
echo "   Job Prefix: release-${VERSION_SHORT}"
echo "   PR: $PR_URL"
echo ""
echo "🎉 Done!"
