#!/bin/bash

set -euo pipefail

# Glo-Grafana Release Script
# Creates release branch and updates tekton pipeline configurations
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   GRAFANA_BRANCH    - Grafana branch name (e.g., release-1.8)
#   GRAFANA_TAG       - Grafana tag (e.g., globalhub-1-8)

# Configuration
GRAFANA_REPO="stolostron/glo-grafana"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$GRAFANA_BRANCH" ] || [ -z "$GRAFANA_TAG" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GRAFANA_BRANCH, GRAFANA_TAG"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Glo-Grafana Release Branch Creation"
echo "================================================"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Grafana Branch: $GRAFANA_BRANCH"
echo "   Grafana Tag: $GRAFANA_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/glo-grafana"
mkdir -p "$WORK_DIR"

if [ ! -d "$REPO_PATH" ]; then
  echo "📥 Cloning $GRAFANA_REPO (--depth=1 for faster clone)..."
  if ! git clone --depth=1 --single-branch --branch main --progress "https://github.com/$GRAFANA_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving" || true; then
    echo "❌ Failed to clone $GRAFANA_REPO"
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

# Extract previous Grafana tag info
PREV_GRAFANA_VERSION=$(echo "$BASE_BRANCH" | sed 's/release-//')
PREV_GRAFANA_TAG="globalhub-$(echo "$PREV_GRAFANA_VERSION" | tr '.' '-')"
echo "Previous Grafana Tag: $PREV_GRAFANA_TAG"

# Check if release branch already exists
if git ls-remote --heads origin "$GRAFANA_BRANCH" | grep -q "$GRAFANA_BRANCH"; then
  echo "ℹ️  Branch $GRAFANA_BRANCH already exists on remote"
  git checkout "$GRAFANA_BRANCH"
else
  echo "🌿 Creating $GRAFANA_BRANCH from origin/$BASE_BRANCH..."
  git checkout -b "$GRAFANA_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $GRAFANA_BRANCH"
fi

# Step 1: Update tekton pipeline files
echo ""
echo "📍 Step 1: Updating tekton pipeline files..."

# Update pull-request pipeline
OLD_PR_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-pull-request.yaml"
NEW_PR_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-pull-request.yaml"

if [ -f "$OLD_PR_PIPELINE" ]; then
  echo "   Renaming and updating pull-request pipeline..."
  git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"

  # Update version references in new file
  sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PR_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PR_PIPELINE"

  echo "   ✅ Updated $NEW_PR_PIPELINE"
else
  echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE"
fi

# Update push pipeline
OLD_PUSH_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-push.yaml"
NEW_PUSH_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-push.yaml"

if [ -f "$OLD_PUSH_PIPELINE" ]; then
  echo "   Renaming and updating push pipeline..."
  git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"

  # Update version references in new file
  sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PUSH_PIPELINE"
  sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PUSH_PIPELINE"

  echo "   ✅ Updated $NEW_PUSH_PIPELINE"
else
  echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE"
fi

# Step 2: Commit changes
echo ""
echo "📍 Step 2: Committing changes on $GRAFANA_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
else
  git add -A

  COMMIT_MSG="Update glo-grafana for ${GRAFANA_BRANCH} (Global Hub ${GH_VERSION})

- Rename and update pull-request pipeline for ${GRAFANA_TAG}
- Rename and update push pipeline for ${GRAFANA_TAG}
- Update branch references to ${GRAFANA_BRANCH}

Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
fi

# Step 3: Push release branch
echo ""
echo "📍 Step 3: Pushing $GRAFANA_BRANCH branch..."

if git push -u origin "$GRAFANA_BRANCH"; then
  echo "   ✅ Pushed branch $GRAFANA_BRANCH to remote"
else
  echo "   ❌ Failed to push branch to remote"
  exit 1
fi

# Summary
echo ""
echo "================================================"
echo "📊 SCRIPT SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Grafana branch: $GRAFANA_BRANCH (from $BASE_BRANCH)"
if [ -n "$PREV_GRAFANA_TAG" ]; then
  echo "  ✓ Renamed tekton pipelines: ${PREV_GRAFANA_TAG} → ${GRAFANA_TAG}"
else
  echo "  ✓ Updated tekton pipelines to ${GRAFANA_TAG}"
fi
echo "  ✓ Branch pushed to remote"
echo ""
echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"
echo ""
echo "Verify branch created:"
echo "  https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
echo ""
echo "After verification:"
echo "  - Check tekton pipelines trigger correctly"
echo "  - Verify grafana images build successfully"
echo ""
echo "================================================"
echo "✅ SUCCESS (3 tasks completed)"
echo "================================================"
