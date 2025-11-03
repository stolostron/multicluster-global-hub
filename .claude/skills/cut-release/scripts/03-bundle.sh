#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Operator Bundle Release Script
# Creates release branch and updates bundle configurations
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   BUNDLE_BRANCH     - Bundle branch name (e.g., release-1.8)
#   BUNDLE_TAG        - Bundle tag (e.g., globalhub-1-8)

# Configuration
BUNDLE_REPO="stolostron/multicluster-global-hub-operator-bundle"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$BUNDLE_BRANCH" ] || [ -z "$BUNDLE_TAG" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, BUNDLE_BRANCH, BUNDLE_TAG"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Multicluster Global Hub Operator Bundle Release"
echo "================================================"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Bundle Branch: $BUNDLE_BRANCH"
echo "   Bundle Tag: $BUNDLE_TAG"
echo ""

# Extract version for display
BUNDLE_VERSION=$(echo "$BUNDLE_BRANCH" | sed 's/release-//')

# Setup repository
REPO_PATH="$WORK_DIR/multicluster-global-hub-operator-bundle"
mkdir -p "$WORK_DIR"

if [ ! -d "$REPO_PATH" ]; then
  echo "📥 Cloning $BUNDLE_REPO (--depth=1 for faster clone)..."
  if ! git clone --depth=1 --single-branch --branch main --progress "https://github.com/$BUNDLE_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving" || true; then
    echo "❌ Failed to clone $BUNDLE_REPO"
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

# Find latest bundle release branch
LATEST_BUNDLE_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

if [ -z "$LATEST_BUNDLE_RELEASE" ]; then
  echo "⚠️  No previous bundle release branch found, using main as base"
  BASE_BRANCH="main"
else
  echo "Latest bundle release detected: $LATEST_BUNDLE_RELEASE"
  BASE_BRANCH="$LATEST_BUNDLE_RELEASE"
fi

# Extract previous bundle tag for replacements
if [ "$BASE_BRANCH" != "main" ]; then
  PREV_BUNDLE_VERSION=$(echo "$BASE_BRANCH" | sed 's/release-//')
  PREV_BUNDLE_TAG="globalhub-$(echo "$PREV_BUNDLE_VERSION" | tr '.' '-')"
  echo "Previous bundle tag: $PREV_BUNDLE_TAG"
else
  PREV_BUNDLE_TAG=""
fi

# Check if new release branch already exists
if git ls-remote --heads origin "$BUNDLE_BRANCH" | grep -q "$BUNDLE_BRANCH"; then
  echo "ℹ️  Branch $BUNDLE_BRANCH already exists on remote"
  git checkout "$BUNDLE_BRANCH"
else
  echo "🌿 Creating $BUNDLE_BRANCH from origin/$BASE_BRANCH..."
  git checkout -b "$BUNDLE_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $BUNDLE_BRANCH"
fi

# Step 1: Update imageDigestMirrorSet (.tekton/images_digest_mirror_set.yaml)
echo ""
echo "📍 Step 1: Updating imageDigestMirrorSet..."

IDMS_FILE=".tekton/images_digest_mirror_set.yaml"
if [ -f "$IDMS_FILE" ]; then
  if [ -n "$PREV_BUNDLE_TAG" ]; then
    echo "   Updating $IDMS_FILE"
    echo "   Changing: multicluster-global-hub-*-${PREV_BUNDLE_TAG}"
    echo "   To:       multicluster-global-hub-*-${BUNDLE_TAG}"

    sed "${SED_INPLACE[@]}" "s/multicluster-global-hub-\([a-z-]*\)-${PREV_BUNDLE_TAG}/multicluster-global-hub-\1-${BUNDLE_TAG}/g" "$IDMS_FILE"
    echo "   ✅ Updated $IDMS_FILE"
  else
    echo "   ⚠️  No previous bundle tag found, skipping update"
  fi
else
  echo "   ⚠️  File not found: $IDMS_FILE"
fi

# Step 2: Update and rename pull-request pipeline
echo ""
echo "📍 Step 2: Updating pull-request pipeline..."

if [ -n "$PREV_BUNDLE_TAG" ]; then
  OLD_PR_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${PREV_BUNDLE_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${BUNDLE_TAG}-pull-request.yaml"

  if [ -f "$OLD_PR_PIPELINE" ]; then
    echo "   Renaming $OLD_PR_PIPELINE to $NEW_PR_PIPELINE"
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"

    echo "   Updating references in $NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PR_PIPELINE"
    echo "   ✅ Renamed and updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Old pull-request pipeline not found: $OLD_PR_PIPELINE"
  fi
else
  echo "   ⚠️  No previous bundle tag found, skipping pipeline update"
fi

# Step 3: Update and rename push pipeline
echo ""
echo "📍 Step 3: Updating push pipeline..."

if [ -n "$PREV_BUNDLE_TAG" ]; then
  OLD_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${PREV_BUNDLE_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${BUNDLE_TAG}-push.yaml"

  if [ -f "$OLD_PUSH_PIPELINE" ]; then
    echo "   Renaming $OLD_PUSH_PIPELINE to $NEW_PUSH_PIPELINE"
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"

    echo "   Updating references in $NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
    echo "   ✅ Renamed and updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Old push pipeline not found: $OLD_PUSH_PIPELINE"
  fi
else
  echo "   ⚠️  No previous bundle tag found, skipping pipeline update"
fi

# Step 4: Update bundle image labels
echo ""
echo "📍 Step 4: Updating bundle image labels..."

# Find bundle manifests (typically in bundle/ or manifests/ directory)
BUNDLE_MANIFESTS=$(find . -name "*.clusterserviceversion.yaml" -o -name "bundle.Dockerfile" 2>/dev/null || true)

if [ -n "$BUNDLE_MANIFESTS" ]; then
  echo "$BUNDLE_MANIFESTS" | while read -r file; do
    if [ -f "$file" ] && [ -n "$PREV_BUNDLE_TAG" ]; then
      if grep -q "$PREV_BUNDLE_VERSION" "$file" 2>/dev/null; then
        echo "   Updating $file"
        sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_VERSION}/${BUNDLE_VERSION}/g" "$file"
        echo "   ✅ Updated version labels in $file"
      fi
    fi
  done
else
  echo "   ⚠️  No bundle manifest files found"
fi

# Step 5: Update konflux-patch.sh
echo ""
echo "📍 Step 5: Updating konflux-patch.sh..."

KONFLUX_SCRIPT="konflux-patch.sh"
if [ -f "$KONFLUX_SCRIPT" ]; then
  if [ -n "$PREV_BUNDLE_TAG" ]; then
    echo "   Updating image references in $KONFLUX_SCRIPT"
    echo "   Changing: *-${PREV_BUNDLE_TAG}"
    echo "   To:       *-${BUNDLE_TAG}"

    sed "${SED_INPLACE[@]}" "s/\([a-z-]*\)-${PREV_BUNDLE_TAG}/\1-${BUNDLE_TAG}/g" "$KONFLUX_SCRIPT"
    echo "   ✅ Updated $KONFLUX_SCRIPT"
  else
    echo "   ⚠️  No previous bundle tag found, skipping update"
  fi
else
  echo "   ⚠️  File not found: $KONFLUX_SCRIPT"
fi

# Step 6: Commit changes
echo ""
echo "📍 Step 6: Committing changes on $BUNDLE_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
else
  git add -A

  COMMIT_MSG="Update bundle for ${BUNDLE_BRANCH} (Global Hub ${GH_VERSION})

- Update imageDigestMirrorSet to use ${BUNDLE_TAG}
- Rename and update pull-request pipeline for ${BUNDLE_TAG}
- Rename and update push pipeline for ${BUNDLE_TAG}
- Update bundle image labels to ${BUNDLE_VERSION}
- Update konflux-patch.sh image references to ${BUNDLE_TAG}

Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
fi

# Step 7: Push release branch
echo ""
echo "📍 Step 7: Pushing $BUNDLE_BRANCH branch..."

if git push -u origin "$BUNDLE_BRANCH"; then
  echo "   ✅ Pushed branch $BUNDLE_BRANCH to remote"
else
  echo "   ❌ Failed to push branch to remote"
  exit 1
fi

# Step 8: Create Pull Request
echo ""
echo "📍 Step 8: Creating Pull Request..."

# Auto-detect GitHub user
GITHUB_USER=$(git remote -v | grep origin | head -1 | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
echo "   GitHub user: $GITHUB_USER"

# Determine base branch for PR (usually main)
PR_BASE_BRANCH="main"

# Create PR body
PR_BODY="Update operator bundle for ${BUNDLE_BRANCH} (Global Hub ${GH_VERSION})

## Changes

- Update imageDigestMirrorSet to use \`${BUNDLE_TAG}\`
- Rename and update pull-request pipeline for \`${BUNDLE_TAG}\`
- Rename and update push pipeline for \`${BUNDLE_TAG}\`
- Update bundle image labels to \`${BUNDLE_VERSION}\`
- Update konflux-patch.sh image references to \`${BUNDLE_TAG}\`

## Version Mapping

- ACM release: \`${RELEASE_BRANCH}\`
- Global Hub version: \`${GH_VERSION}\`
- Bundle branch: \`${BUNDLE_BRANCH}\`
- Bundle tag: \`${BUNDLE_TAG}\`
- Previous bundle: \`${BASE_BRANCH}\`"

# Create PR
PR_URL=$(gh pr create --base "$PR_BASE_BRANCH" --head "$GITHUB_USER:$BUNDLE_BRANCH" \
  --title "Update bundle for ${BUNDLE_BRANCH} (Global Hub ${GH_VERSION})" \
  --body "$PR_BODY" \
  --repo "$BUNDLE_REPO" 2>&1) || true

if [ -n "$PR_URL" ] && [[ ! "$PR_URL" =~ "error" ]]; then
  echo "   ✅ Created PR: $PR_URL"
  PR_CREATED=true
else
  echo "   ⚠️  Failed to create PR automatically"
  echo "   Error: $PR_URL"
  echo "   Please create PR manually from: $GITHUB_USER:$BUNDLE_BRANCH to $BUNDLE_REPO:$PR_BASE_BRANCH"
  PR_CREATED=false
fi

# Summary
echo ""
echo "================================================"
echo "📊 SCRIPT SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Bundle branch: $BUNDLE_BRANCH (from $BASE_BRANCH)"
if [ -f "$IDMS_FILE" ]; then
  echo "  ✓ Updated imageDigestMirrorSet to ${BUNDLE_TAG}"
fi
if [ -n "$PREV_BUNDLE_TAG" ]; then
  echo "  ✓ Renamed tekton pipelines (${PREV_BUNDLE_TAG} → ${BUNDLE_TAG})"
  echo "  ✓ Updated bundle image labels to ${BUNDLE_VERSION}"
  echo "  ✓ Updated konflux-patch.sh image refs"
fi
if [ "$PR_CREATED" = true ]; then
  echo "  ✓ PR to main: Created"
fi
echo ""

if [ "$PR_CREATED" = false ]; then
  echo "❌ FAILED TASKS:"
  echo "  ✗ PR not created"
  echo ""
fi

echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"
echo ""
if [ "$PR_CREATED" = true ]; then
  echo "Review and merge PR:"
  echo "  ${PR_URL}"
  echo ""
  echo "After PR merged:"
  echo "  - Verify bundle images build correctly"
  echo "  - Check tekton pipelines trigger"
else
  echo "Manual PR creation needed:"
  echo "  - Branch: ${GITHUB_USER}:${BUNDLE_BRANCH}"
  echo "  - Target: ${BUNDLE_REPO}:main"
  echo "  - Repository: https://github.com/$BUNDLE_REPO/tree/$BUNDLE_BRANCH"
  echo ""
fi
echo "================================================"
if [ "$PR_CREATED" = true ]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES"
fi
echo "================================================"
