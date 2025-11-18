#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Operator Bundle Release Script
# Supports two modes:
#   CREATE_BRANCHES=true:  Create and push release branch directly to upstream
#   CREATE_BRANCHES=false: Update existing release branch via PR
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   BUNDLE_BRANCH     - Bundle branch name (e.g., release-1.8)
#   BUNDLE_TAG        - Bundle tag (e.g., globalhub-1-8)
#   GITHUB_USER       - GitHub username for PR creation
#   CREATE_BRANCHES          - true: create branch, false: update via PR

# Configuration
BUNDLE_REPO="stolostron/multicluster-global-hub-operator-bundle"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$BUNDLE_BRANCH" || -z "$BUNDLE_TAG" || -z "$GITHUB_USER" || -z "$CREATE_BRANCHES" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, BUNDLE_BRANCH, BUNDLE_TAG, GITHUB_USER, CREATE_BRANCHES"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

# Constants for repeated patterns
readonly NULL_PR_VALUE='null|null'
readonly SEPARATOR_LINE='================================================'

echo "🚀 Operator Bundle Release"
echo "$SEPARATOR_LINE"
echo "   Mode: $([[ "$CREATE_BRANCHES" = true ]] && echo "CUT (create branch)" || echo "UPDATE (PR only)")"
echo "   Release: $RELEASE_BRANCH / $BUNDLE_BRANCH"
echo ""

# Extract version for display
BUNDLE_VERSION="${BUNDLE_BRANCH#release-}"

# Setup repository
REPO_PATH="$WORK_DIR/multicluster-global-hub-operator-bundle"
mkdir -p "$WORK_DIR"

# Remove existing directory for clean clone
if [[ -d "$REPO_PATH" ]]; then
  echo "   Removing existing directory for clean clone..."
  rm -rf "$REPO_PATH"
fi

echo "📥 Cloning $BUNDLE_REPO (--depth=1 for faster clone)..."
git clone --depth=1 --single-branch --branch main --progress "https://github.com/$BUNDLE_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
if [[ ! -d "$REPO_PATH/.git" ]]; then
  echo "❌ Failed to clone $BUNDLE_REPO" >&2
  exit 1
fi
echo "✅ Cloned successfully"

cd "$REPO_PATH"

# Setup user's fork remote
FORK_REPO="git@github.com:${GITHUB_USER}/multicluster-global-hub-operator-bundle.git"
git remote add fork "$FORK_REPO" 2>/dev/null || true

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/multicluster-global-hub-operator-bundle"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/multicluster-global-hub-operator-bundle" >&2
  echo "   Note: Cleanup PR will require manual creation if fork doesn't exist"
fi

# Fetch all release branches
echo "🔄 Fetching release branches..."
git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
echo "   ✅ Release branches fetched"

# Step 0.1: Determine bundle source from multicluster-global-hub
echo ""
echo "📍 Step 0.1: Determining bundle source from multicluster-global-hub..."

MGH_REPO_PATH="${WORK_DIR}/multicluster-global-hub-release"
SOURCE_BUNDLE_DIR=""
BUNDLE_SOURCE_DESCRIPTION=""

# Check if MGH repository exists (script 01 must have run)
if [[ ! -d "$MGH_REPO_PATH/.git" ]]; then
  echo "   ❌ Error: multicluster-global-hub repository not found at $MGH_REPO_PATH" >&2
  echo "   Script 01 must be run first to generate the operator bundle" >&2
  exit 1
fi

cd "$MGH_REPO_PATH"

# Check for PR to main branch (created by script 01)
echo "   Checking for multicluster-global-hub PR to main branch..."
MGH_MAIN_PR=$(gh pr list \
  --repo "stolostron/multicluster-global-hub" \
  --base main \
  --state open \
  --search "Add ${RELEASE_BRANCH} pipeline configurations" \
  --json number,url,headRefName \
  --jq '.[0] | select(. != null) | "\(.number)|\(.url)|\(.headRefName)"' 2>/dev/null || echo "")

if [[ -n "$MGH_MAIN_PR" && "$MGH_MAIN_PR" != "null|null|" ]]; then
  # PR exists - use bundle from PR branch (latest updates before merge)
  PR_NUMBER=$(echo "$MGH_MAIN_PR" | cut -d'|' -f1)
  PR_URL=$(echo "$MGH_MAIN_PR" | cut -d'|' -f2)
  PR_BRANCH=$(echo "$MGH_MAIN_PR" | cut -d'|' -f3)

  echo "   ✅ Found PR #$PR_NUMBER to main: $PR_URL"
  echo "   Branch: $PR_BRANCH"

  # Checkout the PR branch to get the latest bundle
  echo "   Checking out PR branch: $PR_BRANCH"
  git fetch origin "$PR_BRANCH" 2>/dev/null || true
  git checkout "$PR_BRANCH" 2>/dev/null || git checkout -B "$PR_BRANCH" "origin/$PR_BRANCH"

  SOURCE_BUNDLE_DIR="$MGH_REPO_PATH/operator/bundle"
  BUNDLE_SOURCE_DESCRIPTION="PR #$PR_NUMBER branch ($PR_BRANCH)"
  echo "   ✅ Using bundle from PR branch: $PR_BRANCH"
else
  # No PR - use bundle from main branch (normal case after PR is merged)
  echo "   ℹ️  No open PR to main found"
  echo "   Using main branch (normal case after PR merge)..."

  # Checkout main branch
  git fetch origin main 2>/dev/null || true
  git checkout -B main origin/main 2>/dev/null || true

  SOURCE_BUNDLE_DIR="$MGH_REPO_PATH/operator/bundle"
  BUNDLE_SOURCE_DESCRIPTION="main branch"
  echo "   ✅ Using bundle from main branch"
fi

# Verify bundle directory exists
if [[ ! -d "$SOURCE_BUNDLE_DIR" ]]; then
  echo "   ❌ Error: Bundle directory not found at $SOURCE_BUNDLE_DIR" >&2
  exit 1
fi

echo "   ✅ Bundle source: $BUNDLE_SOURCE_DESCRIPTION"
echo "   📁 Path: $SOURCE_BUNDLE_DIR"

# Return to bundle repo
cd "$REPO_PATH"

# Find latest bundle release branch
LATEST_BUNDLE_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

# Check if target branch is the same as latest
if [[ "$LATEST_BUNDLE_RELEASE" = "$BUNDLE_BRANCH" ]]; then
  echo "ℹ️  Target bundle branch is the latest: $BUNDLE_BRANCH"
  echo ""
  echo "   https://github.com/$BUNDLE_REPO/tree/$BUNDLE_BRANCH"
  echo ""
  echo "   Will verify and update if needed..."
  echo ""
fi

if [[ -z "$LATEST_BUNDLE_RELEASE" ]]; then
  echo "⚠️  No previous bundle release branch found, using main as base" >&2
  BASE_BRANCH="main"
else
  echo "Latest bundle release detected: $LATEST_BUNDLE_RELEASE"

  # If target branch is the latest, use second-to-latest as base
  # If target branch is not the latest, use latest as base
  if [[ "$LATEST_BUNDLE_RELEASE" = "$BUNDLE_BRANCH" ]]; then
    # Target is latest - get second-to-latest for base
    SECOND_TO_LATEST=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
      sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
    if [[ -n "$SECOND_TO_LATEST" && "$SECOND_TO_LATEST" != "$BUNDLE_BRANCH" ]]; then
      BASE_BRANCH="$SECOND_TO_LATEST"
      echo "Target is latest release, using previous release as base: $BASE_BRANCH"
    else
      BASE_BRANCH="main"
      echo "No previous release found, using main as base"
    fi
  else
    # Target is not latest - use latest as base
    BASE_BRANCH="$LATEST_BUNDLE_RELEASE"
    echo "Target is older than latest, using latest as base: $BASE_BRANCH"
  fi
fi

# Extract previous bundle tag for replacements
if [[ "$BASE_BRANCH" != "main" ]]; then
  PREV_BUNDLE_VERSION="${BASE_BRANCH#release-}"
  PREV_BUNDLE_TAG="globalhub-${PREV_BUNDLE_VERSION//./-}"
  echo "Previous bundle tag: $PREV_BUNDLE_TAG"
else
  PREV_BUNDLE_TAG=""
fi

# For cleanup PR, we need to find the previous release
# If BUNDLE_BRANCH is the latest, find second-to-latest for cleanup
if [[ "$LATEST_BUNDLE_RELEASE" = "$BUNDLE_BRANCH" ]]; then
  CLEANUP_TARGET_BRANCH=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
    sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
  if [[ -n "$CLEANUP_TARGET_BRANCH" && "$CLEANUP_TARGET_BRANCH" != "$BUNDLE_BRANCH" ]]; then
    echo "Cleanup target: $CLEANUP_TARGET_BRANCH (previous release)"
  else
    CLEANUP_TARGET_BRANCH=""
  fi
else
  # When creating new bundle, BASE_BRANCH is the cleanup target
  CLEANUP_TARGET_BRANCH="$BASE_BRANCH"
fi

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
PR_CREATED=false
PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$BUNDLE_BRANCH" | grep -q "$BUNDLE_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $BUNDLE_BRANCH already exists on origin"
  git fetch origin "$BUNDLE_BRANCH" 2>/dev/null || true
  git checkout -B "$BUNDLE_BRANCH" "origin/$BUNDLE_BRANCH"
else
  if [[ "$CREATE_BRANCHES" != "true" ]]; then
    echo "❌ Error: Branch $BUNDLE_BRANCH does not exist on origin" >&2
    echo "   Run with CREATE_BRANCHES=true to create the branch"
    exit 1
  fi
  echo "🌿 Creating $BUNDLE_BRANCH from origin/$BASE_BRANCH..."
  # Delete local branch if it exists
  git branch -D "$BUNDLE_BRANCH" 2>/dev/null || true
  git checkout -b "$BUNDLE_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $BUNDLE_BRANCH"
fi

echo ""

# Step 0: Copy bundle content from multicluster-global-hub operator
echo "📍 Step 0: Copying bundle content from multicluster-global-hub operator..."
echo "   Source: $BUNDLE_SOURCE_DESCRIPTION"
echo "   Path: $SOURCE_BUNDLE_DIR"
echo "   Target: bundle/"

# Backup existing bundle directory and preserve createdAt timestamps
PRESERVE_CREATED_AT=false
ORIGINAL_CREATED_AT=""
if [[ -d "bundle" ]]; then
  echo "   Backing up existing bundle directory..."
  rm -rf bundle.backup 2>/dev/null || true
  cp -r bundle bundle.backup

  # Extract createdAt timestamp from existing CSV if it exists
  CSV_FILE=$(find bundle/manifests -name "*.clusterserviceversion.yaml" 2>/dev/null | head -1)
  if [[ -n "$CSV_FILE" && -f "$CSV_FILE" ]]; then
    # Extract only the timestamp value, not the entire line
    ORIGINAL_CREATED_AT=$(grep "createdAt:" "$CSV_FILE" | sed -E 's/.*createdAt: "(.*)".*/\1/' | head -1 || echo "")
    if [[ -n "$ORIGINAL_CREATED_AT" ]]; then
      PRESERVE_CREATED_AT=true
      echo "   ℹ️  Preserving original createdAt timestamp: $ORIGINAL_CREATED_AT"
    fi
  fi
fi

# Remove existing bundle content (except .git if it exists)
echo "   Removing old bundle content..."
rm -rf bundle/manifests bundle/metadata bundle/tests 2>/dev/null || true

# Copy new bundle content
echo "   Copying new bundle content..."
mkdir -p bundle

if [[ -d "$SOURCE_BUNDLE_DIR/manifests" ]]; then
  cp -r "$SOURCE_BUNDLE_DIR/manifests" bundle/
  echo "   ✅ Copied manifests/ ($(ls -1 "$SOURCE_BUNDLE_DIR/manifests" | wc -l | tr -d ' ') files)"
fi

if [[ -d "$SOURCE_BUNDLE_DIR/metadata" ]]; then
  cp -r "$SOURCE_BUNDLE_DIR/metadata" bundle/
  echo "   ✅ Copied metadata/"
fi

if [[ -d "$SOURCE_BUNDLE_DIR/tests" ]]; then
  cp -r "$SOURCE_BUNDLE_DIR/tests" bundle/
  echo "   ✅ Copied tests/"
fi

# Restore original createdAt to avoid timestamp-only changes
if [[ "$PRESERVE_CREATED_AT" = true && -n "$ORIGINAL_CREATED_AT" ]]; then
  NEW_CSV_FILE=$(find bundle/manifests -name "*.clusterserviceversion.yaml" 2>/dev/null | head -1)
  if [[ -n "$NEW_CSV_FILE" && -f "$NEW_CSV_FILE" ]]; then
    echo "   Restoring original createdAt timestamp..."
    # Replace only the timestamp value, preserving the line format and indentation
    # Use a simpler sed pattern that works on both Linux and macOS
    sed "${SED_INPLACE[@]}" "s|createdAt: \"[^\"]*\"|createdAt: \"${ORIGINAL_CREATED_AT}\"|" "$NEW_CSV_FILE"
    echo "   ✅ Restored original createdAt: $ORIGINAL_CREATED_AT (avoiding timestamp-only changes)"
  fi
fi

# Stage the copied files
git add bundle/ 2>/dev/null || true

echo "   ✅ Bundle content copied from $BUNDLE_SOURCE_DESCRIPTION"

echo ""

# Step 1: Update imageDigestMirrorSet (.tekton/images_digest_mirror_set.yaml)
echo "📍 Step 1: Updating imageDigestMirrorSet..."

IDMS_FILE=".tekton/images_digest_mirror_set.yaml"
if [[ -f "$IDMS_FILE" ]]; then
  if [[ -n "$PREV_BUNDLE_TAG" ]]; then
    echo "   Updating $IDMS_FILE"
    echo "   Changing: multicluster-global-hub-*-${PREV_BUNDLE_TAG}"
    echo "   To:       multicluster-global-hub-*-${BUNDLE_TAG}"

    sed "${SED_INPLACE[@]}" "s/multicluster-global-hub-\([a-z-]*\)-${PREV_BUNDLE_TAG}/multicluster-global-hub-\1-${BUNDLE_TAG}/g" "$IDMS_FILE"
    echo "   ✅ Updated $IDMS_FILE"
  else
    echo "   ⚠️  No previous bundle tag found, skipping update" >&2
  fi
else
  echo "   ⚠️  File not found: $IDMS_FILE" >&2
fi

# Step 2: Update and rename pull-request pipeline
echo ""
echo "📍 Step 2: Updating pull-request pipeline..."

if [[ -n "$PREV_BUNDLE_TAG" ]]; then
  OLD_PR_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${PREV_BUNDLE_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${BUNDLE_TAG}-pull-request.yaml"

  if [[ "$PREV_BUNDLE_TAG" = "$BUNDLE_TAG" ]]; then
    # Same tag - pipeline should already exist
    if [[ -f "$NEW_PR_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PR_PIPELINE"
      echo "   Skipping modification (may have been updated by other PRs)"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PR_PIPELINE" >&2
    fi
  else
    # Different tag - check if new pipeline already exists
    if [[ -f "$NEW_PR_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PR_PIPELINE"
      echo "   Skipping modification (may have been created by another PR or previous run)"
    elif [[ -f "$OLD_PR_PIPELINE" ]]; then
      # New pipeline doesn't exist, but old one exists locally - rename it
      echo "   Renaming $OLD_PR_PIPELINE to $NEW_PR_PIPELINE"
      git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"

      echo "   Updating references in $NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Renamed and updated $NEW_PR_PIPELINE"
    else
      # Neither new nor old pipeline exists locally - fetch from previous release branch
      echo "   ℹ️  Pipeline not found locally, fetching from origin/$BASE_BRANCH..."

      # Try to fetch from previous release branch
      if git show "origin/$BASE_BRANCH:$OLD_PR_PIPELINE" > "$NEW_PR_PIPELINE" 2>/dev/null; then
        echo "   ✅ Copied from origin/$BASE_BRANCH:$OLD_PR_PIPELINE"

        # Update references in the new file
        sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PR_PIPELINE"
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PR_PIPELINE"
        git add "$NEW_PR_PIPELINE"
        echo "   ✅ Created and updated $NEW_PR_PIPELINE"
      else
        echo "   ❌ Failed to fetch pipeline from origin/$BASE_BRANCH" >&2
      fi
    fi
  fi
else
  echo "   ⚠️  No previous bundle tag found, skipping pipeline update" >&2
fi

# Step 3: Update and rename push pipeline
echo ""
echo "📍 Step 3: Updating push pipeline..."

if [[ -n "$PREV_BUNDLE_TAG" ]]; then
  OLD_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${PREV_BUNDLE_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-bundle-${BUNDLE_TAG}-push.yaml"

  if [[ "$PREV_BUNDLE_TAG" = "$BUNDLE_TAG" ]]; then
    # Same tag - pipeline should already exist
    if [[ -f "$NEW_PUSH_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PUSH_PIPELINE"
      echo "   Skipping modification (may have been updated by other PRs)"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PUSH_PIPELINE" >&2
    fi
  else
    # Different tag - check if new pipeline already exists
    if [[ -f "$NEW_PUSH_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PUSH_PIPELINE"
      echo "   Skipping modification (may have been created by another PR or previous run)"
    elif [[ -f "$OLD_PUSH_PIPELINE" ]]; then
      # New pipeline doesn't exist, but old one exists locally - rename it
      echo "   Renaming $OLD_PUSH_PIPELINE to $NEW_PUSH_PIPELINE"
      git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"

      echo "   Updating references in $NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
      echo "   ✅ Renamed and updated $NEW_PUSH_PIPELINE"
    else
      # Neither new nor old pipeline exists locally - fetch from previous release branch
      echo "   ℹ️  Pipeline not found locally, fetching from origin/$BASE_BRANCH..."

      # Try to fetch from previous release branch
      if git show "origin/$BASE_BRANCH:$OLD_PUSH_PIPELINE" > "$NEW_PUSH_PIPELINE" 2>/dev/null; then
        echo "   ✅ Copied from origin/$BASE_BRANCH:$OLD_PUSH_PIPELINE"

        # Update references in the new file
        sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_TAG}/${BUNDLE_TAG}/g" "$NEW_PUSH_PIPELINE"
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${BUNDLE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
        git add "$NEW_PUSH_PIPELINE"
        echo "   ✅ Created and updated $NEW_PUSH_PIPELINE"
      else
        echo "   ❌ Failed to fetch pipeline from origin/$BASE_BRANCH" >&2
      fi
    fi
  fi
else
  echo "   ⚠️  No previous bundle tag found, skipping pipeline update" >&2
fi

# Step 4: Update bundle image labels
echo ""
echo "📍 Step 4: Updating bundle image labels..."

# Find bundle manifests (typically in bundle/ or manifests/ directory)
BUNDLE_MANIFESTS=$(find . -name "*.clusterserviceversion.yaml" -o -name "bundle.Dockerfile" 2>/dev/null || true)

if [[ -n "$BUNDLE_MANIFESTS" ]]; then
  echo "$BUNDLE_MANIFESTS" | while read -r file; do
    if [[ -f "$file" && -n "$PREV_BUNDLE_TAG" ]]; then
      PREV_BUNDLE_VERSION="${BASE_BRANCH#release-}"
      if grep -q "$PREV_BUNDLE_VERSION" "$file" 2>/dev/null; then
        echo "   Updating $file"
        sed "${SED_INPLACE[@]}" "s/${PREV_BUNDLE_VERSION}/${BUNDLE_VERSION}/g" "$file"
        echo "   ✅ Updated version labels in $file"
      fi
    fi
  done
else
  echo "   ⚠️  No bundle manifest files found" >&2
fi

# Step 4.5: Update CSV skipRange
echo ""
echo "📍 Step 4.5: Updating CSV skipRange..."

CSV_FILE=$(find . -name "*.clusterserviceversion.yaml" 2>/dev/null | head -1)
if [[ -n "$CSV_FILE" && -f "$CSV_FILE" && -n "$PREV_BUNDLE_VERSION" ]]; then
  # Calculate the previous minor version for skipRange
  # Current: 1.7, Previous: 1.6, skipRange should be: ">=1.6.0 <1.7.0"
  CURRENT_MINOR="${BUNDLE_VERSION#*.}"
  CURRENT_MINOR="${CURRENT_MINOR%%.*}"  # Extract minor version (e.g., 7 from 1.7.0)
  PREV_MINOR=$((CURRENT_MINOR - 1))

  EXPECTED_SKIP_RANGE=">=1.${PREV_MINOR}.0 <1.${CURRENT_MINOR}.0"

  echo "   CSV file: $CSV_FILE"
  echo "   Expected skipRange: '$EXPECTED_SKIP_RANGE'"

  # Check current skipRange
  CURRENT_SKIP_RANGE=$(grep "olm.skipRange:" "$CSV_FILE" | sed -E "s/.*olm.skipRange: '(.*)'/\1/")
  echo "   Current skipRange: '$CURRENT_SKIP_RANGE'"

  if [[ "$CURRENT_SKIP_RANGE" != "$EXPECTED_SKIP_RANGE" ]]; then
    echo "   Updating skipRange..."
    # Match the previous pattern and replace with current
    PREV_PREV_MINOR=$((PREV_MINOR - 1))
    sed "${SED_INPLACE[@]}" "s/olm.skipRange: '>=[0-9.]*[0-9] <[0-9.]*[0-9]'/olm.skipRange: '${EXPECTED_SKIP_RANGE}'/" "$CSV_FILE"
    echo "   ✅ Updated skipRange to '${EXPECTED_SKIP_RANGE}'"
  else
    echo "   ✓ skipRange already correct"
  fi
else
  if [[ -z "$CSV_FILE" ]]; then
    echo "   ⚠️  CSV file not found" >&2
  elif [[ -z "$PREV_BUNDLE_VERSION" ]]; then
    echo "   ⚠️  No previous bundle version found, skipping skipRange update" >&2
  fi
fi

# Step 5: Update konflux-patch.sh
echo ""
echo "📍 Step 5: Updating konflux-patch.sh..."

KONFLUX_SCRIPT="konflux-patch.sh"
if [[ -f "$KONFLUX_SCRIPT" ]]; then
  if [[ -n "$PREV_BUNDLE_TAG" ]]; then
    echo "   Updating image references in $KONFLUX_SCRIPT"
    echo "   Changing: *-${PREV_BUNDLE_TAG}"
    echo "   To:       *-${BUNDLE_TAG}"

    sed "${SED_INPLACE[@]}" "s/\([a-z-]*\)-${PREV_BUNDLE_TAG}/\1-${BUNDLE_TAG}/g" "$KONFLUX_SCRIPT"
    echo "   ✅ Updated image references"
  else
    echo "   ⚠️  No previous bundle tag found, skipping image reference update" >&2
  fi

  # Update version replacement in konflux-patch.sh
  if [[ -n "$PREV_BUNDLE_VERSION" ]]; then
    echo "   Updating version replacement in $KONFLUX_SCRIPT"
    echo "   Changing: ${PREV_BUNDLE_VERSION}.0-dev → ${PREV_BUNDLE_VERSION}.0"
    echo "   To:       ${BUNDLE_VERSION}.0-dev → ${BUNDLE_VERSION}.0"

    # Replace the sed command that changes version from X.Y.0-dev to X.Y.0
    sed "${SED_INPLACE[@]}" "s|${PREV_BUNDLE_VERSION}.0-dev\|${PREV_BUNDLE_VERSION}.0|${BUNDLE_VERSION}.0-dev\|${BUNDLE_VERSION}.0|g" "$KONFLUX_SCRIPT"
    echo "   ✅ Updated version replacement"
  else
    echo "   ⚠️  No previous bundle version found, skipping version update" >&2
  fi
else
  echo "   ⚠️  File not found: $KONFLUX_SCRIPT" >&2
fi

# Step 6: Commit changes
echo ""
echo "📍 Step 6: Committing changes on $BUNDLE_BRANCH..."

# Clean up bundle backup before committing
if [[ -d "bundle.backup" ]]; then
  echo "   Cleaning up bundle backup..."
  rm -rf bundle.backup
fi

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  CHANGES_COMMITTED=false
else
  git add -A

  COMMIT_MSG="Update bundle for ${BUNDLE_BRANCH} (Global Hub ${GH_VERSION})

- Copy latest operator bundle from multicluster-global-hub ${BUNDLE_SOURCE_DESCRIPTION}
  (manifests, metadata, tests)
- Update imageDigestMirrorSet to use ${BUNDLE_TAG}
- Rename and update pull-request pipeline for ${BUNDLE_TAG}
- Rename and update push pipeline for ${BUNDLE_TAG}
- Update bundle image labels to ${BUNDLE_VERSION}
- Update CSV skipRange to '>=${PREV_BUNDLE_VERSION}.0 <${BUNDLE_VERSION}.0'
- Update konflux-patch.sh image references to ${BUNDLE_TAG}
- Update konflux-patch.sh version replacement to ${BUNDLE_VERSION}.0

Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}
Bundle source: ${BUNDLE_SOURCE_DESCRIPTION}"

  git commit --signoff -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
  CHANGES_COMMITTED=true
fi

  # Step 7: Push to origin or create PR
  echo ""
  echo "📍 Step 7: Publishing changes..."

  # Check if there are any changes
  if [[ "$CHANGES_COMMITTED" = false ]]; then
    echo "   ℹ️  No changes to publish"
  else
    # Decision: Push directly or create PR based on CREATE_BRANCHES and branch existence
    if [[ "$CREATE_BRANCHES" = "true" && "$BRANCH_EXISTS_ON_ORIGIN" = false ]]; then
      # CUT mode + branch doesn't exist - push directly
      echo "   Pushing new branch $BUNDLE_BRANCH to origin..."
      if git push origin "$BUNDLE_BRANCH" 2>&1; then
        echo "   ✅ Branch pushed to origin: $BUNDLE_REPO/$BUNDLE_BRANCH"
        PUSHED_TO_ORIGIN=true
      else
        echo "   ❌ Failed to push branch to origin" >&2
        exit 1
      fi
    else
      # Branch exists or UPDATE mode - create PR to update it
      echo "   Creating PR to update $BUNDLE_BRANCH..."

      # Check if PR already exists
      echo "   Checking for existing PR to $BUNDLE_BRANCH..."
      EXISTING_PR=$(gh pr list \
        --repo "${BUNDLE_REPO}" \
        --base "$BUNDLE_BRANCH" \
        --state open \
        --search "Update ${BUNDLE_BRANCH} bundle configuration" \
        --json number,url,state,headRefName \
        --jq '.[0] | select(. != null) | "\(.state)|\(.url)|\(.headRefName)"' 2>/dev/null || echo "")

      if [[ -n "$EXISTING_PR" && "$EXISTING_PR" != "$NULL_PR_VALUE" ]]; then
        PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
        PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
        EXISTING_BRANCH=$(echo "$EXISTING_PR" | cut -d'|' -f3)
        echo "   ℹ️  PR already exists (state: $PR_STATE): $PR_URL"
        echo "   Updating existing PR branch: $EXISTING_BRANCH"

        # Push updates to existing PR branch
        if [[ "$FORK_EXISTS" = false ]]; then
          echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
          PR_CREATED=false
        else
          if git push -f fork "$BUNDLE_BRANCH:$EXISTING_BRANCH" 2>&1; then
            echo "   ✅ PR updated with latest changes"
            PR_CREATED=true
          else
            echo "   ⚠️  Failed to push updates to PR" >&2
            PR_CREATED=false
          fi
        fi
      else
        # Create a unique branch name for the PR
        PR_BRANCH="${BUNDLE_BRANCH}-update-$(date +%s)"
        git checkout -b "$PR_BRANCH"

        # Push PR branch to user's fork
        if [[ "$FORK_EXISTS" = false ]]; then
          echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
          echo "   Please fork ${BUNDLE_REPO} to enable PR creation"
          PR_CREATED=false
        else
          echo "   Pushing $PR_BRANCH to fork..."
          if git push -f fork "$PR_BRANCH" 2>&1; then
            echo "   ✅ PR branch pushed to fork"

          # Create PR to the release branch
          PR_BODY="Update ${BUNDLE_BRANCH} bundle configuration

## Changes

- Copy latest operator bundle from multicluster-global-hub (manifests, metadata, tests)
- Update imageDigestMirrorSet to use \`${BUNDLE_TAG}\`
- Rename and update pull-request pipeline for \`${BUNDLE_TAG}\`
- Rename and update push pipeline for \`${BUNDLE_TAG}\`
- Update bundle image labels to \`${BUNDLE_VERSION}\`
- Update CSV skipRange to \`>=${PREV_BUNDLE_VERSION}.0 <${BUNDLE_VERSION}.0\`
- Update konflux-patch.sh image references to \`${BUNDLE_TAG}\`
- Update konflux-patch.sh version replacement to \`${BUNDLE_VERSION}.0\`

## Version Mapping

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${BUNDLE_VERSION}
- **Bundle tag**: ${BUNDLE_TAG}
- **Previous bundle**: ${BASE_BRANCH}"

          PR_CREATE_OUTPUT=$(gh pr create --base "$BUNDLE_BRANCH" --head "${GITHUB_USER}:$PR_BRANCH" \
            --title "Update ${BUNDLE_BRANCH} bundle configuration" \
            --body "$PR_BODY" \
            --repo "$BUNDLE_REPO" 2>&1) || true

          # Check if PR was successfully created
          if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
            PR_URL="$PR_CREATE_OUTPUT"
            echo "   ✅ PR created: $PR_URL"
            PR_CREATED=true
          elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
            PR_URL="${BASH_REMATCH[1]}"
            echo "   ✅ PR created: $PR_URL"
            PR_CREATED=true
          else
            echo "   ⚠️  Failed to create PR" >&2
            echo "   Reason: $PR_CREATE_OUTPUT"
            PR_CREATED=false
          fi
          else
            echo "   ❌ Failed to push PR branch" >&2
          fi
        fi  # End of FORK_EXISTS check for PR
      fi  # End of existing PR check
    fi
  fi

# Step 8: Create cleanup PR to remove GitHub Actions from old release
echo ""
echo "📍 Step 8: Creating cleanup PR for old release..."

CLEANUP_PR_CREATED=false
CLEANUP_PR_URL=""

# Only create cleanup PR if there's a cleanup target (not main)
if [[ "$CLEANUP_TARGET_BRANCH" != "main" && -n "$CLEANUP_TARGET_BRANCH" ]]; then
  echo "   Checking out previous release: $CLEANUP_TARGET_BRANCH..."

  # Clean any uncommitted changes
  git reset --hard HEAD 2>/dev/null || true
  git clean -fd 2>/dev/null || true

  # Checkout old release branch
  git fetch origin "$CLEANUP_TARGET_BRANCH" 2>/dev/null || true
  git checkout -B "$CLEANUP_TARGET_BRANCH" "origin/$CLEANUP_TARGET_BRANCH"

  # Check if GitHub Actions workflow exists
  LABELS_WORKFLOW=".github/workflows/labels.yml"

  if [[ -f "$LABELS_WORKFLOW" ]]; then
    echo "   Found GitHub Actions workflow in $CLEANUP_TARGET_BRANCH"

    # Create cleanup branch
    CLEANUP_BRANCH="cleanup-actions-${CLEANUP_TARGET_BRANCH}-$(date +%s)"
    git checkout -b "$CLEANUP_BRANCH"

    # Remove the workflow file
    git rm "$LABELS_WORKFLOW"

    # Commit the removal
    CLEANUP_COMMIT_MSG="Remove GitHub Actions workflow from ${CLEANUP_TARGET_BRANCH}

Workflow has been moved to ${BUNDLE_BRANCH}.
This prevents duplicate automation on old release branch."

    git commit --signoff -m "$CLEANUP_COMMIT_MSG"
    echo "   ✅ Committed workflow removal"

    # Check if cleanup PR already exists
    echo "   Checking for existing cleanup PR..."
    EXISTING_CLEANUP_PR=$(gh pr list \
      --repo "${BUNDLE_REPO}" \
      --base "$CLEANUP_TARGET_BRANCH" \
      --state all \
      --search "Remove GitHub Actions from ${CLEANUP_TARGET_BRANCH}" \
      --json number,url,state \
      --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

    if [[ -n "$EXISTING_CLEANUP_PR" && "$EXISTING_CLEANUP_PR" != "$NULL_PR_VALUE" ]]; then
      CLEANUP_PR_STATE=$(echo "$EXISTING_CLEANUP_PR" | cut -d'|' -f1)
      CLEANUP_PR_URL=$(echo "$EXISTING_CLEANUP_PR" | cut -d'|' -f2)
      echo "   ℹ️  Cleanup PR already exists (state: $CLEANUP_PR_STATE): $CLEANUP_PR_URL"
      CLEANUP_PR_CREATED=true
    else
      # Push cleanup branch to fork
      if [[ "$FORK_EXISTS" = false ]]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
        echo "   Please fork ${BUNDLE_REPO} and run again, or create cleanup PR manually"
        CLEANUP_PR_CREATED=false
      elif git push -f fork "$CLEANUP_BRANCH" 2>&1; then
        echo "   ✅ Cleanup branch pushed to fork"

        # Create cleanup PR to old release branch
        CLEANUP_PR_BODY="Remove GitHub Actions workflow from ${CLEANUP_TARGET_BRANCH}

The workflow has been moved to the new release branch \`${BUNDLE_BRANCH}\`.

This PR removes the workflow from ${CLEANUP_TARGET_BRANCH} to prevent duplicate automation."

        CLEANUP_PR_OUTPUT=$(gh pr create --base "$CLEANUP_TARGET_BRANCH" --head "${GITHUB_USER}:$CLEANUP_BRANCH" \
          --title "Remove GitHub Actions from ${CLEANUP_TARGET_BRANCH}" \
          --body "$CLEANUP_PR_BODY" \
          --repo "$BUNDLE_REPO" 2>&1) || true

        # Check if cleanup PR was successfully created
        if [[ "$CLEANUP_PR_OUTPUT" =~ ^https:// ]]; then
          CLEANUP_PR_URL="$CLEANUP_PR_OUTPUT"
          echo "   ✅ Cleanup PR created: $CLEANUP_PR_URL"
          CLEANUP_PR_CREATED=true
        elif [[ "$CLEANUP_PR_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
          CLEANUP_PR_URL="${BASH_REMATCH[1]}"
          echo "   ✅ Cleanup PR created: $CLEANUP_PR_URL"
          CLEANUP_PR_CREATED=true
        else
          echo "   ⚠️  Failed to create cleanup PR" >&2
          echo "   Reason: $CLEANUP_PR_OUTPUT"
          CLEANUP_PR_CREATED=false
        fi
      else
        echo "   ⚠️  Failed to push cleanup branch" >&2
        CLEANUP_PR_CREATED=false
      fi
    fi
  else
    echo "   ℹ️  No GitHub Actions workflow found in $CLEANUP_TARGET_BRANCH"
    CLEANUP_PR_CREATED=false
  fi
else
  echo "   ℹ️  No previous release to clean up (cleanup target is $CLEANUP_TARGET_BRANCH)"
  CLEANUP_PR_CREATED=false
fi

# Summary
echo ""
echo "$SEPARATOR_LINE"
echo "📊 WORKFLOW SUMMARY"
echo "$SEPARATOR_LINE"
echo "Release: $RELEASE_BRANCH / $BUNDLE_BRANCH"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Bundle branch: $BUNDLE_BRANCH (from $BASE_BRANCH)"
if [[ "$CHANGES_COMMITTED" = true ]]; then
  echo "  ✓ Copied operator bundle from: $BUNDLE_SOURCE_DESCRIPTION"
  echo "  ✓ Updated imageDigestMirrorSet to ${BUNDLE_TAG}"
fi
if [[ -n "$PREV_BUNDLE_TAG" ]]; then
  echo "  ✓ Renamed tekton pipelines (${PREV_BUNDLE_TAG} → ${BUNDLE_TAG})"
  echo "  ✓ Updated bundle image labels to ${BUNDLE_VERSION}"
  echo "  ✓ Updated CSV skipRange to '>=${PREV_BUNDLE_VERSION} <${BUNDLE_VERSION}'"
  echo "  ✓ Updated konflux-patch.sh image refs and version replacement"
fi
if [[ "$PUSHED_TO_ORIGIN" = true ]]; then
  echo "  ✓ Pushed to origin: ${BUNDLE_REPO}/${BUNDLE_BRANCH}"
fi
if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
  echo "  ✓ PR to $BUNDLE_BRANCH: ${PR_URL}"
fi
if [[ "$CLEANUP_PR_CREATED" = true && -n "$CLEANUP_PR_URL" ]]; then
  echo "  ✓ Cleanup PR to $CLEANUP_TARGET_BRANCH: ${CLEANUP_PR_URL}"
fi
echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"
if [[ "$PUSHED_TO_ORIGIN" = true ]]; then
  echo "✅ Branch pushed to origin successfully"
  echo ""
  echo "Branch: https://github.com/$BUNDLE_REPO/tree/$BUNDLE_BRANCH"
  echo ""
  echo "Verify: Bundle images and tekton pipelines"
elif [[ "$PR_CREATED" = true || "$CLEANUP_PR_CREATED" = true ]]; then
  PR_COUNT=0
  if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
    PR_COUNT=$((PR_COUNT + 1))
    echo "${PR_COUNT}. Review and merge PR to $BUNDLE_BRANCH:"
    echo "   ${PR_URL}"
  fi
  if [[ "$CLEANUP_PR_CREATED" = true && -n "$CLEANUP_PR_URL" ]]; then
    PR_COUNT=$((PR_COUNT + 1))
    echo "${PR_COUNT}. Review and merge cleanup PR to $CLEANUP_TARGET_BRANCH:"
    echo "   ${CLEANUP_PR_URL}"
  fi
  echo ""
  echo "After merge: Verify bundle images and tekton pipelines"
else
  echo "Repository: https://github.com/$BUNDLE_REPO/tree/$BUNDLE_BRANCH"
fi
echo ""
echo "$SEPARATOR_LINE"
if [[ "$PUSHED_TO_ORIGIN" = true || "$PR_CREATED" = true ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES" >&2
fi
echo "$SEPARATOR_LINE"
