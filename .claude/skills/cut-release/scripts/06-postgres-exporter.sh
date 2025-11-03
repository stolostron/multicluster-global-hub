#!/bin/bash

set -euo pipefail

# Postgres Exporter Release Script
# Creates release branch and updates tekton pipeline configurations
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   POSTGRES_TAG      - Postgres tag (e.g., globalhub-1-8)

# Configuration
POSTGRES_REPO="stolostron/postgres_exporter"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$POSTGRES_TAG" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, POSTGRES_TAG"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Postgres Exporter Release Branch Creation"
echo "================================================"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Postgres Tag: $POSTGRES_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/postgres_exporter"
mkdir -p "$WORK_DIR"

if [ ! -d "$REPO_PATH" ]; then
  echo "📥 Cloning $POSTGRES_REPO (--depth=1 for faster clone)..."
  if ! git clone --depth=1 --single-branch --branch main --progress "https://github.com/$POSTGRES_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving" || true; then
    echo "❌ Failed to clone $POSTGRES_REPO"
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
  echo "⚠️  No previous release branch found, using main as base"
  BASE_BRANCH="main"
else
  echo "Latest release detected: $LATEST_RELEASE"
  BASE_BRANCH="$LATEST_RELEASE"
fi

# Extract previous Postgres tag from base branch
if [ "$BASE_BRANCH" != "main" ]; then
  PREV_VERSION=$(echo "$BASE_BRANCH" | sed 's/release-//')
  PREV_MINOR=$(echo "$PREV_VERSION" | cut -d. -f2)
  PREV_GH_MINOR=$((PREV_MINOR - 9))
  PREV_POSTGRES_TAG="globalhub-1-${PREV_GH_MINOR}"
  echo "Previous Postgres Tag: $PREV_POSTGRES_TAG"
else
  PREV_POSTGRES_TAG=""
fi

# Check if release branch already exists
if git ls-remote --heads origin "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  echo "ℹ️  Branch $RELEASE_BRANCH already exists on remote"
  git checkout "$RELEASE_BRANCH"
else
  echo "🌿 Creating $RELEASE_BRANCH from origin/$BASE_BRANCH..."
  git checkout -b "$RELEASE_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $RELEASE_BRANCH"
fi

# Step 1: Update tekton pipeline files
echo ""
echo "📍 Step 1: Updating tekton pipeline files..."

if [ -n "$PREV_POSTGRES_TAG" ]; then
  # Update pull-request pipeline
  OLD_PR_PIPELINE=".tekton/postgres-exporter-${PREV_POSTGRES_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/postgres-exporter-${POSTGRES_TAG}-pull-request.yaml"

  if [ -f "$OLD_PR_PIPELINE" ]; then
    echo "   Renaming and updating pull-request pipeline..."
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"

    # Update version references in new file
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PR_PIPELINE"

    echo "   ✅ Updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE"
  fi

  # Update push pipeline
  OLD_PUSH_PIPELINE=".tekton/postgres-exporter-${PREV_POSTGRES_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/postgres-exporter-${POSTGRES_TAG}-push.yaml"

  if [ -f "$OLD_PUSH_PIPELINE" ]; then
    echo "   Renaming and updating push pipeline..."
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"

    # Update version references in new file
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PUSH_PIPELINE"

    echo "   ✅ Updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE"
  fi
else
  echo "   ℹ️  No previous Postgres tag found, skipping pipeline updates"
fi

# Step 2: Commit changes
echo ""
echo "📍 Step 2: Committing changes on $RELEASE_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
else
  git add -A

  COMMIT_MSG="Update postgres_exporter for ${RELEASE_BRANCH} (Global Hub ${GH_VERSION})

- Rename and update pull-request pipeline for ${POSTGRES_TAG}
- Rename and update push pipeline for ${POSTGRES_TAG}
- Update branch references to ${RELEASE_BRANCH}

Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
fi

# Step 3: Push release branch
echo ""
echo "📍 Step 3: Pushing $RELEASE_BRANCH branch..."

if git push -u origin "$RELEASE_BRANCH"; then
  echo "   ✅ Pushed branch $RELEASE_BRANCH to remote"
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
echo "  ✓ Postgres branch: $RELEASE_BRANCH (from $BASE_BRANCH)"
if [ -n "$PREV_POSTGRES_TAG" ]; then
  echo "  ✓ Renamed tekton pipelines: ${PREV_POSTGRES_TAG} → ${POSTGRES_TAG}"
else
  echo "  ✓ Created branch (no pipeline updates needed)"
fi
echo "  ✓ Branch pushed to remote"
echo ""
echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"
echo ""
echo "Verify branch created:"
echo "  https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
echo ""
echo "After verification:"
echo "  - Check tekton pipelines trigger correctly"
echo "  - Verify postgres exporter images build"
echo ""
echo "================================================"
echo "✅ SUCCESS"
echo "================================================"
