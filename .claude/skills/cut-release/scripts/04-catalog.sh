#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Operator Catalog Release Script
# Creates release branch and updates all catalog configurations automatically
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   CATALOG_BRANCH    - Catalog branch name (e.g., release-1.8)
#   CATALOG_TAG       - Catalog tag (e.g., globalhub-1-8)
#   OCP_MIN           - Minimum OCP version number (e.g., 416)
#   OCP_MAX           - Maximum OCP version number (e.g., 420)

# Configuration
CATALOG_REPO="stolostron/multicluster-global-hub-operator-catalog"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$CATALOG_BRANCH" ] || [ -z "$CATALOG_TAG" ] || [ -z "$OCP_MIN" ] || [ -z "$OCP_MAX" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, CATALOG_BRANCH, CATALOG_TAG, OCP_MIN, OCP_MAX"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Multicluster Global Hub Operator Catalog Release"
echo "================================================"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Catalog Branch: $CATALOG_BRANCH"
echo "   Catalog Tag: $CATALOG_TAG"
echo "   OCP Versions: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"
echo ""

# Extract version for display
CATALOG_VERSION=$(echo "$CATALOG_BRANCH" | sed 's/release-//')

# Setup repository
REPO_PATH="$WORK_DIR/multicluster-global-hub-operator-catalog"
mkdir -p "$WORK_DIR"

if [ ! -d "$REPO_PATH" ]; then
  echo "📥 Cloning $CATALOG_REPO (--depth=1 for faster clone)..."
  if ! git clone --depth=1 --single-branch --branch main --progress "https://github.com/$CATALOG_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving" || true; then
    echo "❌ Failed to clone $CATALOG_REPO"
    exit 1
  fi
  echo "✅ Cloned successfully"
else
  echo "ℹ️  Using existing clone at $REPO_PATH"
fi

cd "$REPO_PATH"

# Fetch latest changes and update to latest commit
echo "🔄 Fetching latest changes..."
if git fetch origin; then
  echo "   Updating to latest commit..."
  git checkout main 2>/dev/null || git checkout master 2>/dev/null || true
  git pull origin main 2>/dev/null || git pull origin master 2>/dev/null || true
  echo "   ✅ Updated to latest commit"
else
  echo "   ⚠️  Failed to fetch, continuing with existing state"
fi

# Find latest release branch
LATEST_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

if [ -z "$LATEST_RELEASE" ]; then
  echo "❌ Error: No previous release branch found"
  exit 1
fi

echo "Latest release detected: $LATEST_RELEASE"
BASE_BRANCH="$LATEST_RELEASE"

# Extract previous catalog info
PREV_CATALOG_VERSION=$(echo "$BASE_BRANCH" | sed 's/release-//')
PREV_CATALOG_TAG="globalhub-${PREV_CATALOG_VERSION//./-}"
PREV_MINOR=$(echo "$PREV_CATALOG_VERSION" | sed 's/1\.//')
# Calculate previous OCP range using same formula as main script: OCP_BASE=10, range of 5 versions
OCP_BASE=10
PREV_OCP_MIN=$((4*100 + OCP_BASE + PREV_MINOR))
PREV_OCP_MAX=$((PREV_OCP_MIN + 4))

echo "Previous catalog: $PREV_CATALOG_VERSION (OCP 4.$((PREV_OCP_MIN%100)) - 4.$((PREV_OCP_MAX%100)))"
echo ""

# Check if new release branch already exists
if git ls-remote --heads origin "$CATALOG_BRANCH" | grep -q "$CATALOG_BRANCH"; then
  echo "ℹ️  Branch $CATALOG_BRANCH already exists on remote"
  git checkout "$CATALOG_BRANCH"
else
  echo "🌿 Creating $CATALOG_BRANCH from origin/$BASE_BRANCH..."
  git checkout -b "$CATALOG_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $CATALOG_BRANCH"
fi

# Step 1: Update images-mirror-set.yaml
echo ""
echo "📍 Step 1: Updating images-mirror-set.yaml..."

IMAGES_MIRROR_FILE=".tekton/images-mirror-set.yaml"
if [ -f "$IMAGES_MIRROR_FILE" ]; then
  echo "   Updating image references in $IMAGES_MIRROR_FILE"
  echo "   Changing: *-$PREV_CATALOG_TAG"
  echo "   To:       *-$CATALOG_TAG"

  # Replace all occurrences of previous tag with new tag
  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$IMAGES_MIRROR_FILE"

  # Update OCP version references
  for ((ocp_ver=PREV_OCP_MIN%100; ocp_ver<=PREV_OCP_MAX%100; ocp_ver++)); do
    new_ocp=$((ocp_ver + 1))
    sed "${SED_INPLACE[@]}" "s/v4${ocp_ver}/v4${new_ocp}/g" "$IMAGES_MIRROR_FILE"
  done

  echo "   ✅ Updated $IMAGES_MIRROR_FILE"
else
  echo "   ⚠️  File not found: $IMAGES_MIRROR_FILE"
fi

# Step 2: Create new OCP pipeline files and remove old ones
echo ""
echo "📍 Step 2: Updating OCP pipeline files..."

# Process new OCP versions (remove oldest, add newest)
NEW_OCP_VER=$((OCP_MAX%100))
OLD_OCP_VER=$((PREV_OCP_MIN%100))

echo "   Adding OCP 4.${NEW_OCP_VER} pipelines..."
echo "   Removing OCP 4.${OLD_OCP_VER} pipelines..."

# Copy and update pull-request pipeline for new OCP version
LATEST_PR_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}-pull-request.yaml"
NEW_PR_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4${NEW_OCP_VER}-${CATALOG_TAG}-pull-request.yaml"

if [ -f "$LATEST_PR_PIPELINE" ]; then
  cp "$LATEST_PR_PIPELINE" "$NEW_PR_PIPELINE"

  # Update version references in new file
  sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PR_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/release-catalog-$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}/release-catalog-${NEW_OCP_VER}-${CATALOG_TAG}/g" "$NEW_PR_PIPELINE"

  echo "   ✅ Created $NEW_PR_PIPELINE"
fi

# Copy and update push pipeline for new OCP version
LATEST_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}-push.yaml"
NEW_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4${NEW_OCP_VER}-${CATALOG_TAG}-push.yaml"

if [ -f "$LATEST_PUSH_PIPELINE" ]; then
  cp "$LATEST_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"

  # Update version references in new file
  sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PUSH_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/release-catalog-$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}/release-catalog-${NEW_OCP_VER}-${CATALOG_TAG}/g" "$NEW_PUSH_PIPELINE"

  echo "   ✅ Created $NEW_PUSH_PIPELINE"
fi

# Update existing OCP version pipelines
echo "   Updating existing OCP version pipelines..."
for ((ocp_ver=(OCP_MIN%100); ocp_ver<NEW_OCP_VER; ocp_ver++)); do
  prev_ocp=$((ocp_ver))

  OLD_PR=".tekton/multicluster-global-hub-operator-catalog-v4${prev_ocp}-${PREV_CATALOG_TAG}-pull-request.yaml"
  NEW_PR=".tekton/multicluster-global-hub-operator-catalog-v4${prev_ocp}-${CATALOG_TAG}-pull-request.yaml"

  OLD_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${prev_ocp}-${PREV_CATALOG_TAG}-push.yaml"
  NEW_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${prev_ocp}-${CATALOG_TAG}-push.yaml"

  if [ -f "$OLD_PR" ]; then
    git mv "$OLD_PR" "$NEW_PR" 2>/dev/null || cp "$OLD_PR" "$NEW_PR"
    sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR"
    echo "   ✅ Updated v4${prev_ocp} pull-request pipeline"
  fi

  if [ -f "$OLD_PUSH" ]; then
    git mv "$OLD_PUSH" "$NEW_PUSH" 2>/dev/null || cp "$OLD_PUSH" "$NEW_PUSH"
    sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH"
    echo "   ✅ Updated v4${prev_ocp} push pipeline"
  fi
done

# Remove old OCP version pipelines
OLD_OCP_PR=".tekton/multicluster-global-hub-operator-catalog-v4${OLD_OCP_VER}-${PREV_CATALOG_TAG}-pull-request.yaml"
OLD_OCP_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${OLD_OCP_VER}-${PREV_CATALOG_TAG}-push.yaml"

if [ -f "$OLD_OCP_PR" ]; then
  git rm "$OLD_OCP_PR" 2>/dev/null || rm "$OLD_OCP_PR"
  echo "   ✅ Removed old OCP 4.${OLD_OCP_VER} pull-request pipeline"
fi

if [ -f "$OLD_OCP_PUSH" ]; then
  git rm "$OLD_OCP_PUSH" 2>/dev/null || rm "$OLD_OCP_PUSH"
  echo "   ✅ Removed old OCP 4.${OLD_OCP_VER} push pipeline"
fi

# Step 3: Update README.md
echo ""
echo "📍 Step 3: Updating README.md..."

README_FILE="README.md"
if [ -f "$README_FILE" ]; then
  echo "   Updating version references in $README_FILE"

  # Update Global Hub version references
  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_VERSION}/${CATALOG_VERSION}/g" "$README_FILE"
  sed "${SED_INPLACE[@]}" "s/v1\.${PREV_MINOR}\.0/v1.${CATALOG_MINOR}.0/g" "$README_FILE"

  # Update OCP version ranges
  sed "${SED_INPLACE[@]}" "s/4\.$((PREV_OCP_MIN%100))/4.$((OCP_MIN%100))/g" "$README_FILE"
  sed "${SED_INPLACE[@]}" "s/4\.$((PREV_OCP_MAX%100))/4.$((OCP_MAX%100))/g" "$README_FILE"

  echo "   ✅ Updated $README_FILE"
else
  echo "   ⚠️  File not found: $README_FILE"
fi

# Step 4: Update GitHub Actions workflow
echo ""
echo "📍 Step 4: Updating GitHub Actions workflow..."

LABELS_WORKFLOW=".github/workflows/labels.yml"
if [ -f "$LABELS_WORKFLOW" ]; then
  echo "   Updating $LABELS_WORKFLOW for new release branch"

  # Update branch references
  sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$LABELS_WORKFLOW"

  echo "   ✅ Updated $LABELS_WORKFLOW"
else
  echo "   ⚠️  File not found: $LABELS_WORKFLOW"
fi

# Step 5: Commit all changes
echo ""
echo "📍 Step 5: Committing changes on $CATALOG_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
else
  git add -A

  COMMIT_MSG="Update catalog for ${CATALOG_BRANCH} (Global Hub ${GH_VERSION})

- Update images-mirror-set.yaml to use ${CATALOG_TAG}
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Remove OCP 4.${OLD_OCP_VER} pipelines
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines
- Update README.md with new version information
- Update GitHub Actions workflow for ${CATALOG_BRANCH}

Supports OCP 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
fi

# Step 6: Push release branch
echo ""
echo "📍 Step 6: Pushing $CATALOG_BRANCH branch..."

if git push -u origin "$CATALOG_BRANCH"; then
  echo "   ✅ Pushed branch $CATALOG_BRANCH to remote"
else
  echo "   ❌ Failed to push branch to remote"
  exit 1
fi

# Step 7: Create Pull Request for new branch
echo ""
echo "📍 Step 7: Creating Pull Request for new release..."

# Auto-detect GitHub user
GITHUB_USER=$(git remote -v | grep origin | head -1 | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
echo "   GitHub user: $GITHUB_USER"

PR_BASE_BRANCH="main"

PR_BODY="Update catalog for ${CATALOG_BRANCH} (Global Hub ${GH_VERSION})

## Changes

- Update images-mirror-set.yaml to use \`${CATALOG_TAG}\`
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Remove OCP 4.${OLD_OCP_VER} pipelines (end of support)
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines for new Global Hub version
- Update README.md with new release information
- Update GitHub Actions workflow for new release branch

## Version Mapping

- ACM release: \`${RELEASE_BRANCH}\`
- Global Hub version: \`${GH_VERSION}\`
- Catalog branch: \`${CATALOG_BRANCH}\`
- Supported OCP versions: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
- Previous release: \`${BASE_BRANCH}\`"

PR_URL=$(gh pr create --base "$PR_BASE_BRANCH" --head "$GITHUB_USER:$CATALOG_BRANCH" \
  --title "Update catalog for ${CATALOG_BRANCH} (Global Hub ${GH_VERSION})" \
  --body "$PR_BODY" \
  --repo "$CATALOG_REPO" 2>&1) || true

if [ -n "$PR_URL" ] && [[ ! "$PR_URL" =~ "error" ]]; then
  echo "   ✅ Created PR: $PR_URL"
  PR_CREATED=true
else
  echo "   ⚠️  Failed to create PR automatically"
  echo "   Error: $PR_URL"
  echo "   Please create PR manually from: $GITHUB_USER:$CATALOG_BRANCH to $CATALOG_REPO:$PR_BASE_BRANCH"
  PR_CREATED=false
fi

# Step 8: Create PR to remove GitHub Actions from old branch
echo ""
echo "📍 Step 8: Creating PR to remove GitHub Actions from previous release..."

# Switch to previous release branch
git checkout "$BASE_BRANCH"

# Check if workflow file exists
if [ -f "$LABELS_WORKFLOW" ]; then
  # Create a cleanup branch
  CLEANUP_BRANCH="cleanup-actions-${BASE_BRANCH}"
  git checkout -b "$CLEANUP_BRANCH"

  # Remove the workflow file
  git rm "$LABELS_WORKFLOW"

  git commit -m "Remove GitHub Actions workflow from ${BASE_BRANCH}

Workflow has been moved to ${CATALOG_BRANCH}.
This prevents duplicate automation on old release branch."

  # Push cleanup branch
  if git push -u origin "$CLEANUP_BRANCH"; then
    echo "   ✅ Pushed cleanup branch"

    # Create cleanup PR
    CLEANUP_PR_BODY="Remove GitHub Actions workflow from ${BASE_BRANCH}

The workflow has been moved to the new release branch \`${CATALOG_BRANCH}\`.

This PR removes the workflow from ${BASE_BRANCH} to prevent duplicate automation."

    CLEANUP_PR_URL=$(gh pr create --base "$BASE_BRANCH" --head "$GITHUB_USER:$CLEANUP_BRANCH" \
      --title "Remove GitHub Actions from ${BASE_BRANCH}" \
      --body "$CLEANUP_PR_BODY" \
      --repo "$CATALOG_REPO" 2>&1) || true

    if [ -n "$CLEANUP_PR_URL" ] && [[ ! "$CLEANUP_PR_URL" =~ "error" ]]; then
      echo "   ✅ Created cleanup PR: $CLEANUP_PR_URL"
      CLEANUP_PR_CREATED=true
    else
      echo "   ⚠️  Failed to create cleanup PR"
      CLEANUP_PR_CREATED=false
    fi
  else
    echo "   ⚠️  Failed to push cleanup branch"
    CLEANUP_PR_CREATED=false
  fi
else
  echo "   ℹ️  No GitHub Actions workflow to remove from ${BASE_BRANCH}"
  CLEANUP_PR_CREATED=false
fi

# Summary
echo ""
echo "================================================"
echo "📊 SCRIPT SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo "Supported OCP: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Catalog branch: $CATALOG_BRANCH"
echo "  ✓ Updated images-mirror-set.yaml to ${CATALOG_TAG}"
echo "  ✓ Added OCP 4.${NEW_OCP_VER} pipelines"
echo "  ✓ Removed OCP 4.${OLD_OCP_VER} pipelines"
echo "  ✓ Updated OCP 4.$((OCP_MIN%100+1)) - 4.$((OCP_MAX%100-1)) pipelines"
echo "  ✓ Updated README.md"
echo "  ✓ Updated GitHub Actions workflow"
if [ "$PR_CREATED" = true ]; then
  echo "  ✓ PR to main: Created"
fi
if [ "$CLEANUP_PR_CREATED" = true ]; then
  echo "  ✓ Cleanup PR to ${BASE_BRANCH}: Created"
fi
echo ""

# Show failures
if [ "$PR_CREATED" = false ] || [ "$CLEANUP_PR_CREATED" = false ]; then
  echo "❌ FAILED TASKS:"
  if [ "$PR_CREATED" = false ]; then
    echo "  ✗ PR to main not created"
  fi
  if [ "$CLEANUP_PR_CREATED" = false ] && [ -f "$BASE_GITHUB_WORKFLOW" ]; then
    echo "  ✗ Cleanup PR to ${BASE_BRANCH} not created"
  fi
  echo ""
fi

echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"
echo ""
if [ "$PR_CREATED" = true ]; then
  echo "Review and merge PRs:"
  echo "  1. New release PR to main:"
  echo "     ${PR_URL}"
  if [ "$CLEANUP_PR_CREATED" = true ]; then
    echo "  2. Cleanup PR to ${BASE_BRANCH}:"
    echo "     ${CLEANUP_PR_URL}"
  fi
  echo ""
  echo "After PRs merged:"
  echo "  - Verify OCP 4.${NEW_OCP_VER} pipelines work"
  echo "  - Confirm OCP 4.${OLD_OCP_VER} pipelines removed"
else
  echo "Manual PR creation needed:"
  echo "  - Branch: ${GITHUB_USER}:${CATALOG_BRANCH}"
  echo "  - Target: ${CATALOG_REPO}:main"
  echo "  - Repository: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
fi
echo ""
echo "================================================"
if [ "$PR_CREATED" = true ]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES"
fi
echo "================================================"
