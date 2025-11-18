#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Repository Release Script
# Implements complete release workflow:
# 1. Update configurations on main branch (new .tekton files with target_branch="main")
# 2. Create release branch from updated main
# 3. Create PR to main with new release configurations
# 4. Update previous release .tekton files to point to their release branch
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   GH_VERSION_SHORT  - Short Global Hub version (e.g., 1.8)
#   ACM_VERSION       - ACM version (e.g., 2.17)

# Configuration
REPO_ORG="${REPO_ORG:-stolostron}"
REPO_NAME="${REPO_NAME:-multicluster-global-hub}"

# Use GITHUB_USER from cut-release.sh (already auto-detected)
FORK_USER="${GITHUB_USER}"

# Fork configuration - user's fork repository
FORK_URL="git@github.com:${FORK_USER}/hub-of-hubs.git"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$GH_VERSION_SHORT" || -z "$ACM_VERSION" || -z "$CREATE_BRANCHES" || -z "$GITHUB_USER" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GH_VERSION_SHORT, ACM_VERSION, CREATE_BRANCHES, GITHUB_USER"
  exit 1
fi

# Always use temporary directory for clean clone
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"
REPO_PATH="${WORK_DIR}/multicluster-global-hub-release"

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS requires -i with empty string
  SED_INPLACE=(-i "")
else
  # Linux uses -i without argument
  SED_INPLACE=(-i)
fi

# Constants for repeated patterns
readonly NULL_PR_VALUE='null|null'
readonly SEPARATOR_LINE='================================================'
readonly TARGET_BRANCH_GREP_PATTERN='target_branch =='
readonly TARGET_BRANCH_EXTRACT_SED='s/.*target_branch == "([^"]+)".*/\1/'

echo "🚀 Multicluster Global Hub Release Workflow"
echo "$SEPARATOR_LINE"
echo "   Mode: $([[ "$CREATE_BRANCHES" = true ]] && echo "CUT (create branches)" || echo "UPDATE (PR only)")"
echo ""
echo "Workflow:"
echo "  1. Update main branch with new .tekton files (target_branch=main)"
if [[ "$CREATE_BRANCHES" = "true" ]]; then
  echo "  2. Create release branch from updated main (CUT MODE)"
else
  echo "  2. Skip release branch creation (UPDATE MODE)"
fi
echo "  3. Create PR to upstream main with new release configurations"
echo "  4. Update previous release .tekton files (target_branch=previous_release_branch)"
echo "  5. Update current release .tekton files (target_branch=current_release_branch)"
echo "$SEPARATOR_LINE"

# Step 1: Clone repository and setup remotes
echo ""
echo "📍 Step 1: Cloning repository..."

# Create work directory and remove existing repo for clean clone
mkdir -p "$WORK_DIR"
if [[ -d "$REPO_PATH" ]]; then
  echo "   Removing existing repository directory..."
  rm -rf "$REPO_PATH"
fi

# Clone from upstream with shallow clone for speed
echo "   Cloning ${REPO_ORG}/${REPO_NAME} (--depth=1 for faster clone)..."
git clone --depth=1 --single-branch --branch main --progress "https://github.com/${REPO_ORG}/${REPO_NAME}.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
cd "$REPO_PATH"

# Setup remotes: origin (fork) and upstream (stolostron)
echo "   Setting up git remotes..."
# Rename the cloned remote from 'origin' to 'upstream'
git remote rename origin upstream

# Add user's fork as 'origin'
git remote add origin "$FORK_URL"

# Verify remote configuration
ORIGIN_URL=$(git remote get-url origin)
UPSTREAM_URL=$(git remote get-url upstream)
echo "   ✅ Repository cloned and configured:"
echo "      origin (fork): $ORIGIN_URL"
echo "      upstream: $UPSTREAM_URL"
echo "   ✅ Repository ready at $REPO_PATH"

# Step 2: Calculate previous release version based on current release
echo ""
echo "📍 Step 2: Calculating previous release version..."

# Fetch release branches from upstream
REMOTE="upstream"
echo "   Fetching release branches from upstream..."
git fetch upstream 'refs/heads/release-*:refs/remotes/upstream/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true

# Calculate previous release based on current release
# For release-2.16, previous should be release-2.15 (not 2.17)
CURRENT_ACM_MAJOR=$(echo "$ACM_VERSION" | cut -d. -f1)
CURRENT_ACM_MINOR=$(echo "$ACM_VERSION" | cut -d. -f2)
PREV_ACM_MINOR=$((CURRENT_ACM_MINOR - 1))
PREV_RELEASE_BRANCH="release-${CURRENT_ACM_MAJOR}.${PREV_ACM_MINOR}"

# Calculate previous Global Hub version
PREV_GH_MINOR=$((PREV_ACM_MINOR - 9))
PREV_GH_VERSION_SHORT="1.${PREV_GH_MINOR}"

echo "   Current:  $RELEASE_BRANCH / release-${GH_VERSION_SHORT}"
echo "   Previous: $PREV_RELEASE_BRANCH / release-${PREV_GH_VERSION_SHORT}"

# Step 3: Prepare working branch for main PR
echo ""
echo "📍 Step 3: Preparing branch for PR to main..."

# Clean any uncommitted changes and reset to clean state
git reset --hard HEAD 2>/dev/null || true
git clean -fd 2>/dev/null || true

# Ensure we're on latest main
git checkout main 2>/dev/null || git checkout master 2>/dev/null || true
git reset --hard "$REMOTE/main" 2>/dev/null || git reset --hard "$REMOTE/master" 2>/dev/null || true

# Create working branch for main PR
MAIN_PR_BRANCH="release-${ACM_VERSION}-tekton-configs"
echo "   Creating branch for main PR: $MAIN_PR_BRANCH"

# Check if this branch already exists on remote
if git ls-remote --heads "$REMOTE" "$MAIN_PR_BRANCH" | grep -q "$MAIN_PR_BRANCH"; then
  echo "   Branch exists on $REMOTE, fetching latest..."
  git fetch "$REMOTE" "$MAIN_PR_BRANCH:$MAIN_PR_BRANCH" 2>/dev/null || true
  git checkout -B "$MAIN_PR_BRANCH" "$REMOTE/$MAIN_PR_BRANCH"
else
  git checkout -B "$MAIN_PR_BRANCH" "$REMOTE/main"
fi

# Step 4: Create new .tekton/ configuration files for main branch
echo ""
echo "📍 Step 4: Creating new .tekton/ configuration files..."

TEKTON_UPDATED=false

if [[ ! -d ".tekton" ]]; then
  echo "   ⚠️  .tekton/ directory not found in current repository" >&2
else
  # Define version tags
  PREV_TAG="globalhub-${PREV_GH_VERSION_SHORT//./-}"
  NEW_TAG="globalhub-${GH_VERSION_SHORT//./-}"

  echo "   Processing $NEW_TAG files..."

  # Process each component (agent, manager, operator)
  for component in agent manager operator; do
    for pipeline_type in pull-request push; do
      OLD_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"
      NEW_FILE=".tekton/multicluster-global-hub-${component}-${NEW_TAG}-${pipeline_type}.yaml"

      # Determine expected target_branch based on pipeline type
      if [[ "$pipeline_type" = "pull-request" ]]; then
        EXPECTED_TARGET="main"
      else
        # push pipelines should target the release branch
        EXPECTED_TARGET="$RELEASE_BRANCH"
      fi

      # Check if new file already exists
      if [[ -f "$NEW_FILE" ]]; then
        echo "   ℹ️  Already exists: $NEW_FILE"

        # Check current target_branch
        CURRENT_TARGET=$(grep "$TARGET_BRANCH_GREP_PATTERN" "$NEW_FILE" | sed -E "$TARGET_BRANCH_EXTRACT_SED" || echo "")

        if [[ "$CURRENT_TARGET" = "$EXPECTED_TARGET" ]]; then
          echo "   ✓ Content verified: target_branch=$EXPECTED_TARGET"
          TEKTON_UPDATED=true
        elif [[ -n "$CURRENT_TARGET" ]]; then
          # Update target_branch to expected value
          echo "   ⚠️  Updating target_branch: $CURRENT_TARGET -> $EXPECTED_TARGET" >&2
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$NEW_FILE"
          git add "$NEW_FILE"
          echo "   ✅ Updated: $NEW_FILE (target_branch=$EXPECTED_TARGET)"
          TEKTON_UPDATED=true
        else
          echo "   ⚠️  Cannot find target_branch in $NEW_FILE" >&2
        fi
        continue
      fi

      # Old file exists, copy to new file
      if [[ -f "$OLD_FILE" ]]; then
        # Copy old file to new file
        cp "$OLD_FILE" "$NEW_FILE"
        echo "   ✅ Created: $NEW_FILE (from $OLD_FILE)"

        # Update content in the new file
        # 1. Update application and component labels
        sed "${SED_INPLACE[@]}" "s/release-${PREV_TAG}/release-${NEW_TAG}/g" "$NEW_FILE"
        sed "${SED_INPLACE[@]}" "s/${component}-${PREV_TAG}/${component}-${NEW_TAG}/g" "$NEW_FILE"

        # 2. Update name
        sed "${SED_INPLACE[@]}" "s/name: multicluster-global-hub-${component}-${PREV_TAG}/name: multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        # 3. Update service account name
        sed "${SED_INPLACE[@]}" "s/build-pipeline-multicluster-global-hub-${component}-${PREV_TAG}/build-pipeline-multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        # 4. Update target_branch to expected value
        # First get the current target_branch from the copied file
        CURRENT_TARGET=$(grep "$TARGET_BRANCH_GREP_PATTERN" "$NEW_FILE" | sed -E "$TARGET_BRANCH_EXTRACT_SED" || echo "")
        if [[ -n "$CURRENT_TARGET" && "$CURRENT_TARGET" != "$EXPECTED_TARGET" ]]; then
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$NEW_FILE"
        fi

        # Verify target_branch is set correctly
        if ! grep -q "target_branch == \"${EXPECTED_TARGET}\"" "$NEW_FILE"; then
          echo "   ⚠️  Warning: target_branch not set to $EXPECTED_TARGET in $NEW_FILE" >&2
        fi

        git add "$NEW_FILE"
        echo "   ✅ Updated content in $NEW_FILE (target_branch=$EXPECTED_TARGET)"
        TEKTON_UPDATED=true
      else
        echo "   ⚠️  Source file not found: $OLD_FILE" >&2
      fi
    done
  done

  if [[ "$TEKTON_UPDATED" = true ]]; then
    echo "   ✅ Tekton files updated"

    # Remove previous release pipeline files
    echo ""
    echo "   Removing previous release pipeline files..."
    PREV_TAG="globalhub-${PREV_GH_VERSION_SHORT//./-}"

    for component in agent manager operator; do
      for pipeline_type in pull-request push; do
        OLD_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"
        if [[ -f "$OLD_FILE" ]]; then
          git rm "$OLD_FILE" 2>/dev/null || rm -f "$OLD_FILE"
          echo "   ✅ Removed: $OLD_FILE"
        fi
      done
    done
  else
    echo "   ⚠️  No updates needed" >&2
  fi
fi

# Step 5: Update Containerfile.* version labels (idempotent)
echo ""
echo "📍 Step 5: Updating Containerfile.* version labels..."

CONTAINERFILE_UPDATED=false

# Update specific Containerfile locations
for component in agent manager operator; do
  CONTAINERFILE="${component}/Containerfile.${component}"

  if [[ -f "$CONTAINERFILE" ]]; then
    echo "   Processing $CONTAINERFILE..."

    # Check if already updated to new version (idempotent)
    if grep -q "LABEL version=\"release-${GH_VERSION_SHORT}\"" "$CONTAINERFILE"; then
      echo "   ℹ️  Already at correct version: release-${GH_VERSION_SHORT}"
      CONTAINERFILE_UPDATED=true
    # Update from previous version
    elif grep -q "LABEL version=\"release-${PREV_GH_VERSION_SHORT}\"" "$CONTAINERFILE"; then
      sed "${SED_INPLACE[@]}" "s/LABEL version=\"release-${PREV_GH_VERSION_SHORT}\"/LABEL version=\"release-${GH_VERSION_SHORT}\"/" "$CONTAINERFILE"
      echo "   ✅ Updated version label: release-${PREV_GH_VERSION_SHORT} -> release-${GH_VERSION_SHORT}"
      CONTAINERFILE_UPDATED=true
    else
      # Try to find any version label and report
      VERSION_LINE=$(grep "LABEL version=" "$CONTAINERFILE" 2>/dev/null || echo "")
      if [[ -n "$VERSION_LINE" ]]; then
        echo "   ⚠️  Found: $VERSION_LINE" >&2
        echo "   ⚠️  Expected: release-${PREV_GH_VERSION_SHORT} or release-${GH_VERSION_SHORT}" >&2
        echo "   ⚠️  Manual update may be needed" >&2
      else
        echo "   ℹ️  No version label found in $CONTAINERFILE"
      fi
    fi
  else
    echo "   ⚠️  File not found: $CONTAINERFILE" >&2
  fi
done

if [[ "$CONTAINERFILE_UPDATED" = true ]]; then
  echo "   ✅ Containerfile labels updated"
else
  echo "   ⚠️  No updates needed" >&2
fi

# Step 5.5: Update GitHub workflow files
echo ""
echo "📍 Step 5.5: Updating GitHub workflow files..."

WORKFLOW_UPDATED=false
WORKFLOW_FILE=".github/workflows/go.yml"

if [[ -f "$WORKFLOW_FILE" ]]; then
  # Update bundle branch reference from previous to current version
  if grep -q "multicluster-global-hub-operator-bundle.git -b release-${PREV_GH_VERSION_SHORT}" "$WORKFLOW_FILE"; then
    sed "${SED_INPLACE[@]}" "s|multicluster-global-hub-operator-bundle.git -b release-${PREV_GH_VERSION_SHORT}|multicluster-global-hub-operator-bundle.git -b release-${GH_VERSION_SHORT}|g" "$WORKFLOW_FILE"
    echo "   ✅ Updated bundle branch: release-${PREV_GH_VERSION_SHORT} -> release-${GH_VERSION_SHORT}"
    WORKFLOW_UPDATED=true
  elif grep -q "multicluster-global-hub-operator-bundle.git -b release-${GH_VERSION_SHORT}" "$WORKFLOW_FILE"; then
    echo "   ℹ️  Bundle branch already set to release-${GH_VERSION_SHORT}"
    WORKFLOW_UPDATED=true
  else
    # Try to find any bundle branch reference
    CURRENT_BUNDLE_BRANCH=$(grep "multicluster-global-hub-operator-bundle.git -b" "$WORKFLOW_FILE" | sed -E 's/.*-b (release-[0-9.]+).*/\1/' | head -1 || echo "")
    if [[ -n "$CURRENT_BUNDLE_BRANCH" ]]; then
      echo "   ⚠️  Found unexpected bundle branch: $CURRENT_BUNDLE_BRANCH" >&2
      echo "   ⚠️  Expected: release-${PREV_GH_VERSION_SHORT} or release-${GH_VERSION_SHORT}" >&2
      # Update anyway
      sed "${SED_INPLACE[@]}" "s|multicluster-global-hub-operator-bundle.git -b ${CURRENT_BUNDLE_BRANCH}|multicluster-global-hub-operator-bundle.git -b release-${GH_VERSION_SHORT}|g" "$WORKFLOW_FILE"
      echo "   ✅ Updated bundle branch: $CURRENT_BUNDLE_BRANCH -> release-${GH_VERSION_SHORT}"
      WORKFLOW_UPDATED=true
    else
      echo "   ⚠️  No bundle branch reference found in $WORKFLOW_FILE" >&2
    fi
  fi
else
  echo "   ⚠️  File not found: $WORKFLOW_FILE" >&2
fi

# Step 5.6: Update renovate.json baseBranches
echo ""
echo "📍 Step 5.6: Updating renovate.json baseBranches..."

RENOVATE_UPDATED=false
RENOVATE_FILE="renovate.json"

if [[ -f "$RENOVATE_FILE" ]]; then
  # Calculate the branches to keep:
  # - main (always)
  # - previous release (PREV_RELEASE_BRANCH)
  # - previous-1 release
  # - previous-2 release
  PREV_MINUS_1_ACM=$((PREV_ACM_MINOR - 1))
  PREV_MINUS_2_ACM=$((PREV_ACM_MINOR - 2))

  PREV_MINUS_1_BRANCH="release-${CURRENT_ACM_MAJOR}.${PREV_MINUS_1_ACM}"
  PREV_MINUS_2_BRANCH="release-${CURRENT_ACM_MAJOR}.${PREV_MINUS_2_ACM}"

  # New baseBranches: ["main", "release-X.Y", "release-X.Y-1", "release-X.Y-2"]
  NEW_BASE_BRANCHES="[\"main\", \"${PREV_RELEASE_BRANCH}\", \"${PREV_MINUS_1_BRANCH}\", \"${PREV_MINUS_2_BRANCH}\"]"

  echo "   Target baseBranches: $NEW_BASE_BRANCHES"

  # Check current baseBranches
  CURRENT_BASE_BRANCHES=$(grep '"baseBranches"' "$RENOVATE_FILE" | sed -E 's/.*"baseBranches": (\[.*\]).*/\1/' || echo "")
  echo "   Current baseBranches: $CURRENT_BASE_BRANCHES"

  if [[ "$CURRENT_BASE_BRANCHES" = "$NEW_BASE_BRANCHES" ]]; then
    echo "   ℹ️  baseBranches already correct"
  else
    # Update baseBranches
    sed "${SED_INPLACE[@]}" "s|\"baseBranches\": \[.*\]|\"baseBranches\": ${NEW_BASE_BRANCHES}|" "$RENOVATE_FILE"
    echo "   ✅ Updated baseBranches to: $NEW_BASE_BRANCHES"
    RENOVATE_UPDATED=true
  fi
else
  echo "   ⚠️  File not found: $RENOVATE_FILE" >&2
fi

# Step 5.7: Update operator Makefile
echo ""
echo "📍 Step 5.7: Updating operator/Makefile..."

MAKEFILE_UPDATED=false
OPERATOR_MAKEFILE="operator/Makefile"
if [[ -f "$OPERATOR_MAKEFILE" ]]; then
  # Update VERSION
  if grep -q "VERSION ?= ${PREV_GH_VERSION_SHORT}.0-dev" "$OPERATOR_MAKEFILE"; then
    sed "${SED_INPLACE[@]}" "s/VERSION ?= ${PREV_GH_VERSION_SHORT}.0-dev/VERSION ?= ${GH_VERSION_SHORT}.0-dev/g" "$OPERATOR_MAKEFILE"
    echo "   ✅ Updated VERSION: ${PREV_GH_VERSION_SHORT}.0-dev -> ${GH_VERSION_SHORT}.0-dev"
    MAKEFILE_UPDATED=true
  else
    echo "   ℹ️  VERSION already set or different pattern"
  fi

  # Update CHANNELS
  if grep -q "CHANNELS = \"release-${PREV_GH_VERSION_SHORT}\"" "$OPERATOR_MAKEFILE"; then
    sed "${SED_INPLACE[@]}" "s/CHANNELS = \"release-${PREV_GH_VERSION_SHORT}\"/CHANNELS = \"release-${GH_VERSION_SHORT}\"/g" "$OPERATOR_MAKEFILE"
    echo "   ✅ Updated CHANNELS: release-${PREV_GH_VERSION_SHORT} -> release-${GH_VERSION_SHORT}"
    MAKEFILE_UPDATED=true
  else
    echo "   ℹ️  CHANNELS already set or different pattern"
  fi

  # Update DEFAULT_CHANNEL
  if grep -q "DEFAULT_CHANNEL = \"release-${PREV_GH_VERSION_SHORT}\"" "$OPERATOR_MAKEFILE"; then
    sed "${SED_INPLACE[@]}" "s/DEFAULT_CHANNEL = \"release-${PREV_GH_VERSION_SHORT}\"/DEFAULT_CHANNEL = \"release-${GH_VERSION_SHORT}\"/g" "$OPERATOR_MAKEFILE"
    echo "   ✅ Updated DEFAULT_CHANNEL: release-${PREV_GH_VERSION_SHORT} -> release-${GH_VERSION_SHORT}"
    MAKEFILE_UPDATED=true
  else
    echo "   ℹ️  DEFAULT_CHANNEL already set or different pattern"
  fi
else
  echo "   ⚠️  File not found: $OPERATOR_MAKEFILE" >&2
fi

# Step 5.8: Update CSV (skipRange, ACM version, maturity)
echo ""
echo "📍 Step 5.8: Updating CSV (skipRange, ACM version, maturity)..."

CSV_UPDATED=false
CSV_FILE="operator/config/manifests/bases/multicluster-global-hub-operator.clusterserviceversion.yaml"
if [[ -f "$CSV_FILE" ]]; then
  CURRENT_MINOR="${GH_VERSION_SHORT#*.}"
  PREV_MINOR=$((CURRENT_MINOR - 1))

  EXPECTED_SKIP_RANGE=">=1.${PREV_MINOR}.0 <1.${CURRENT_MINOR}.0"

  echo "   Expected skipRange: $EXPECTED_SKIP_RANGE"

  # Check current skipRange
  CURRENT_SKIP_RANGE=$(grep "olm.skipRange:" "$CSV_FILE" | sed -E "s/.*olm.skipRange: '(.*)'/\1/" || echo "")
  echo "   Current skipRange: '$CURRENT_SKIP_RANGE'"

  if [[ "$CURRENT_SKIP_RANGE" != "$EXPECTED_SKIP_RANGE" ]]; then
    sed "${SED_INPLACE[@]}" "s/olm.skipRange: '>=[0-9.]*[0-9] <[0-9.]*[0-9]'/olm.skipRange: '${EXPECTED_SKIP_RANGE}'/g" "$CSV_FILE"
    echo "   ✅ Updated skipRange to: $EXPECTED_SKIP_RANGE"
    CSV_UPDATED=true
  else
    echo "   ✓ skipRange already correct"
  fi

  # Update ACM version in documentation URL
  PREV_ACM_VERSION="${CURRENT_ACM_MAJOR}.${PREV_ACM_MINOR}"

  echo ""
  echo "   Updating ACM version in documentation URL..."
  if grep -q "red_hat_advanced_cluster_management_for_kubernetes/${PREV_ACM_VERSION}" "$CSV_FILE"; then
    sed "${SED_INPLACE[@]}" "s|red_hat_advanced_cluster_management_for_kubernetes/${PREV_ACM_VERSION}|red_hat_advanced_cluster_management_for_kubernetes/${ACM_VERSION}|g" "$CSV_FILE"
    echo "   ✅ Updated ACM version: ${PREV_ACM_VERSION} -> ${ACM_VERSION}"
    CSV_UPDATED=true
  elif grep -q "red_hat_advanced_cluster_management_for_kubernetes/${ACM_VERSION}" "$CSV_FILE"; then
    echo "   ℹ️  ACM version already set to ${ACM_VERSION}"
  else
    # Try to find any ACM version
    CURRENT_ACM_VER=$(grep "red_hat_advanced_cluster_management_for_kubernetes/" "$CSV_FILE" | sed -E 's|.*red_hat_advanced_cluster_management_for_kubernetes/([0-9.]+).*|\1|' | head -1 || echo "")
    if [[ -n "$CURRENT_ACM_VER" ]]; then
      echo "   ⚠️  Found unexpected ACM version: $CURRENT_ACM_VER" >&2
      echo "   ⚠️  Expected: ${PREV_ACM_VERSION} or ${ACM_VERSION}" >&2
      # Update anyway
      sed "${SED_INPLACE[@]}" "s|red_hat_advanced_cluster_management_for_kubernetes/${CURRENT_ACM_VER}|red_hat_advanced_cluster_management_for_kubernetes/${ACM_VERSION}|g" "$CSV_FILE"
      echo "   ✅ Updated ACM version: $CURRENT_ACM_VER -> ${ACM_VERSION}"
      CSV_UPDATED=true
    else
      echo "   ⚠️  No ACM version reference found in $CSV_FILE" >&2
    fi
  fi

  # Update maturity field (Global Hub release version)
  echo ""
  echo "   Updating maturity field..."
  if grep -q "maturity: release-${PREV_GH_VERSION_SHORT}" "$CSV_FILE"; then
    sed "${SED_INPLACE[@]}" "s/maturity: release-${PREV_GH_VERSION_SHORT}/maturity: release-${GH_VERSION_SHORT}/" "$CSV_FILE"
    echo "   ✅ Updated maturity: release-${PREV_GH_VERSION_SHORT} -> release-${GH_VERSION_SHORT}"
    CSV_UPDATED=true
  elif grep -q "maturity: release-${GH_VERSION_SHORT}" "$CSV_FILE"; then
    echo "   ℹ️  Maturity already set to release-${GH_VERSION_SHORT}"
  else
    # Try to find any maturity value
    CURRENT_MATURITY=$(grep "maturity:" "$CSV_FILE" | sed -E 's/.*maturity: (.*)/\1/' | head -1 || echo "")
    if [[ -n "$CURRENT_MATURITY" ]]; then
      echo "   ⚠️  Found unexpected maturity: $CURRENT_MATURITY" >&2
      echo "   ⚠️  Expected: release-${PREV_GH_VERSION_SHORT} or release-${GH_VERSION_SHORT}" >&2
      # Update anyway
      sed "${SED_INPLACE[@]}" "s/maturity: ${CURRENT_MATURITY}/maturity: release-${GH_VERSION_SHORT}/" "$CSV_FILE"
      echo "   ✅ Updated maturity: $CURRENT_MATURITY -> release-${GH_VERSION_SHORT}"
      CSV_UPDATED=true
    else
      echo "   ⚠️  No maturity field found in $CSV_FILE" >&2
    fi
  fi
else
  echo "   ⚠️  File not found: $CSV_FILE" >&2
fi

# Step 5.9: Generate operator bundle
echo ""
echo "📍 Step 5.9: Generating operator bundle..."

# Only run if Makefile or CSV was updated
if [[ "$MAKEFILE_UPDATED" = true || "$CSV_UPDATED" = true ]]; then
  if [[ -d "operator" ]]; then
    cd operator

    echo "   Running: make generate"
    if make generate >/dev/null 2>&1; then
      echo "   ✅ make generate completed"
    else
      echo "   ⚠️  make generate had warnings (may be expected)"
    fi

    echo "   Running: make fmt"
    if make fmt >/dev/null 2>&1; then
      echo "   ✅ make fmt completed"
    else
      echo "   ⚠️  make fmt had warnings (may be expected)"
    fi

    echo "   Running: make bundle"
    if make bundle >/dev/null 2>&1; then
      echo "   ✅ make bundle completed"
    else
      echo "   ⚠️  make bundle had warnings (may be expected)"
    fi

    cd ..
    echo "   ✅ Operator bundle generated"
  else
    echo "   ⚠️  operator directory not found" >&2
  fi
else
  echo "   ℹ️  Skipping bundle generation (Makefile and CSV unchanged)"
fi

# Step 6: Commit changes for main branch PR
echo ""
echo "📍 Step 6: Committing changes for main branch..."

# Check for staged and unstaged changes
if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  MAIN_CHANGES_COMMITTED=false
else
  # Stage all changes (including operator bundle generated files)
  git add .tekton/ 2>/dev/null || true
  git add -- */Containerfile.* 2>/dev/null || true
  git add .github/workflows/ 2>/dev/null || true
  git add renovate.json 2>/dev/null || true
  git add operator/ 2>/dev/null || true

  git commit --signoff -m "Add ${RELEASE_BRANCH} pipeline configurations

- Add new .tekton/ pipelines for ${NEW_TAG} (target_branch=main)
- Remove previous release pipeline files (${PREV_TAG})
- Update Containerfile version labels to release-${GH_VERSION_SHORT}
- Update GitHub workflow bundle branch to release-${GH_VERSION_SHORT}
- Update renovate.json baseBranches (maintain main + last 3 releases)
- Update operator Makefile (VERSION, CHANNELS, DEFAULT_CHANNEL)
- Update CSV (skipRange, ACM version, maturity)
- Regenerate operator bundle

ACM: ${RELEASE_BRANCH}, Global Hub: release-${GH_VERSION_SHORT}"

  echo "   ✅ Changes committed to $MAIN_PR_BRANCH"
  MAIN_CHANGES_COMMITTED=true
fi

# Step 7: Create release branch from updated main
echo ""
echo "📍 Step 7: Creating release branch ${RELEASE_BRANCH}..."

# Determine remote (upstream if available, otherwise origin)
if git remote | grep -q "^upstream$"; then
  UPSTREAM_REMOTE="upstream"
else
  UPSTREAM_REMOTE="origin"
fi

# Check if release branch already exists on remote
RELEASE_BRANCH_EXISTS=false
if git ls-remote --heads "$UPSTREAM_REMOTE" "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  RELEASE_BRANCH_EXISTS=true
  echo "   ℹ️  Branch $RELEASE_BRANCH already exists on $UPSTREAM_REMOTE"
fi

# Create/checkout release branch from the PR branch (which has latest changes)
if [[ "$RELEASE_BRANCH_EXISTS" = true ]]; then
  echo "   ℹ️  Release branch already exists, skipping creation"
  echo "   Note: Release branch exists on upstream, no merge needed"
  # Just checkout to continue with other steps
  git checkout -B "$RELEASE_BRANCH" "$UPSTREAM_REMOTE/$RELEASE_BRANCH"
else
  if [[ "$CREATE_BRANCHES" != "true" ]]; then
    echo "   ℹ️  Release branch does not exist - skipping (UPDATE mode)"
    echo "   Note: Run with CREATE_BRANCHES=true to create the release branch"
  else
    echo "   Creating new release branch from $MAIN_PR_BRANCH..."
    git checkout -b "$RELEASE_BRANCH" "$MAIN_PR_BRANCH"
    echo "   ✅ Created release branch: $RELEASE_BRANCH"
  fi
fi

# Extract GitHub usernames from remotes
ORIGIN_USER=$(git remote get-url origin | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')
UPSTREAM_USER=$(git remote get-url upstream | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')

echo "   ✅ Fork workflow: PRs from origin/${ORIGIN_USER} -> upstream/${UPSTREAM_USER}"
echo "   ℹ️  Release branch ${RELEASE_BRANCH} managed by upstream maintainers"

RELEASE_BRANCH_PUSHED=false

# Step 8: Push PR branch and create PR to main
echo ""
echo "📍 Step 8: Creating PR to main branch..."

# Clean any uncommitted changes before proceeding
git reset --hard HEAD 2>/dev/null || true
git clean -fd 2>/dev/null || true

# Check if PR already exists
echo "   Checking for existing PR to main..."

# Use fork workflow PR head format
PR_HEAD="${ORIGIN_USER}:${MAIN_PR_BRANCH}"
echo "   PR will be from: ${PR_HEAD} -> ${REPO_ORG}:main"

EXISTING_MAIN_PR=$(gh pr list \
  --repo "${REPO_ORG}/${REPO_NAME}" \
  --head "${PR_HEAD}" \
  --base main \
  --state all \
  --json number,url,state \
  --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

# Check if PR branch has any differences from upstream/main before pushing
git checkout "$MAIN_PR_BRANCH"

# Compare PR branch with upstream/main to see if there are actual differences
echo "   Checking if PR branch differs from upstream/main..."
git fetch "$UPSTREAM_REMOTE" main >/dev/null 2>&1

if git diff --quiet "$MAIN_PR_BRANCH" "$UPSTREAM_REMOTE/main"; then
  echo "   ℹ️  PR branch is identical to upstream/main - no PR needed"
  MAIN_CHANGES_COMMITTED=false
  PUSH_SUCCESS=false
else
  echo "   ✅ PR branch has changes compared to upstream/main"

  # Push PR branch to origin (your fork) - only if there are differences
  if [[ "$MAIN_CHANGES_COMMITTED" = true ]]; then
    echo "   Pushing $MAIN_PR_BRANCH to origin (${ORIGIN_USER})..."

    # Push to origin (force push is safe for your own fork)
    if git push -f origin "$MAIN_PR_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin"
      PUSH_SUCCESS=true
    else
      echo "   ⚠️  Failed to push branch to origin" >&2
      PUSH_SUCCESS=false
    fi
  else
    # Should not happen, but handle gracefully
    PUSH_SUCCESS=false
  fi
fi

if [[ "$PUSH_SUCCESS" = true ]]; then
  # Check if PR exists
  if [[ -n "$EXISTING_MAIN_PR" && "$EXISTING_MAIN_PR" != "$NULL_PR_VALUE" ]]; then
    MAIN_PR_STATE=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f1)
    MAIN_PR_URL=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f2)
    echo "   ✅ PR already exists and updated (state: $MAIN_PR_STATE): $MAIN_PR_URL"
    MAIN_PR_CREATED=true
  else
    # Create new PR to main
    echo "   Creating PR to ${REPO_ORG}:main..."
    PR_CREATE_OUTPUT=$(gh pr create \
      --repo "${REPO_ORG}/${REPO_NAME}" \
      --base main \
      --head "${PR_HEAD}" \
      --title "Add ${RELEASE_BRANCH} tekton pipelines and update configurations" \
      --body "## Summary

Add new pipeline configurations for ${RELEASE_BRANCH} to the main branch.

## Changes

- Add new .tekton/ pipeline files for \`${NEW_TAG}\`:
  - \`*-pull-request.yaml\`: \`target_branch=main\`
  - \`*-push.yaml\`: \`target_branch=${RELEASE_BRANCH}\`
- Remove previous release pipeline files (\`${PREV_TAG}\`)
- Update Containerfile version labels to \`release-${GH_VERSION_SHORT}\`
- Update GitHub workflow bundle branch to \`release-${GH_VERSION_SHORT}\`
- Update renovate.json baseBranches (maintain main + last 3 releases)
- Update operator Makefile (VERSION, CHANNELS, DEFAULT_CHANNEL)
- Update CSV (skipRange, ACM version, maturity)
- Regenerate operator bundle

## Release Info

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${GH_VERSION_SHORT}

## Note

This PR updates all necessary configurations for the new release on the main branch." 2>&1) || true

    # Check if PR was successfully created or already exists
    if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
      MAIN_PR_URL="$PR_CREATE_OUTPUT"
      echo "   ✅ PR created: $MAIN_PR_URL"
      MAIN_PR_CREATED=true
    elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
      # PR already exists, extract URL from error message
      MAIN_PR_URL="${BASH_REMATCH[1]}"
      echo "   ✅ PR already exists and updated: $MAIN_PR_URL"
      MAIN_PR_CREATED=true
    else
      echo "   ⚠️  Failed to create PR automatically" >&2
      echo "   Reason: $PR_CREATE_OUTPUT"
      echo "   ℹ️  You can create the PR manually at:"
      echo "      https://github.com/${REPO_ORG}/${REPO_NAME}/compare/main...${PR_HEAD}"
      MAIN_PR_CREATED=false
    fi
  fi
else
  echo "   ℹ️  No changes to push - branch is up to date with main"
  MAIN_PR_CREATED=false
fi

# Step 9: Update previous release .tekton files to point to their release branch
echo ""
echo "📍 Step 9: Updating previous release .tekton files..."

# Clean any uncommitted changes before proceeding
git reset --hard HEAD 2>/dev/null || true
git clean -fd 2>/dev/null || true

# Previous release branch was already calculated in Step 2
# PREV_RELEASE_BRANCH is already set to release-${CURRENT_ACM_MAJOR}.${PREV_ACM_MINOR}

# Check if previous release branch exists
PREV_RELEASE_EXISTS=false
if git ls-remote --heads "$UPSTREAM_REMOTE" "$PREV_RELEASE_BRANCH" | grep -q "$PREV_RELEASE_BRANCH"; then
  PREV_RELEASE_EXISTS=true
  echo "   ✓ Previous release branch exists: $PREV_RELEASE_BRANCH"
else
  echo "   ⚠️  Previous release branch not found: $PREV_RELEASE_BRANCH" >&2
  echo "   Skipping previous release update"
fi

PREV_RELEASE_UPDATED=false

if [[ "$PREV_RELEASE_EXISTS" = true ]]; then
  # Checkout previous release branch
  echo "   Checking out $PREV_RELEASE_BRANCH..."
  git fetch "$UPSTREAM_REMOTE" "$PREV_RELEASE_BRANCH" >/dev/null 2>&1
  git checkout -B "$PREV_RELEASE_BRANCH" "$UPSTREAM_REMOTE/$PREV_RELEASE_BRANCH"

  # Update the previous version .tekton files to point to previous release branch
  PREV_TAG="globalhub-${PREV_GH_VERSION_SHORT//./-}"

  echo "   Updating ${PREV_TAG} files to target_branch=${PREV_RELEASE_BRANCH}..."

  for component in agent manager operator; do
    for pipeline_type in pull-request push; do
      PREV_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"

      if [[ -f "$PREV_FILE" ]]; then
        # Check current target_branch
        CURRENT_TARGET=$(grep "$TARGET_BRANCH_GREP_PATTERN" "$PREV_FILE" | sed -E "$TARGET_BRANCH_EXTRACT_SED" || echo "")

        if [[ "$CURRENT_TARGET" = "$PREV_RELEASE_BRANCH" ]]; then
          echo "   ℹ️  Already correct: $PREV_FILE (target_branch=$PREV_RELEASE_BRANCH)"
        elif [[ "$CURRENT_TARGET" = "main" ]]; then
          # Update from main to release branch
          sed "${SED_INPLACE[@]}" "s/target_branch == \"main\"/target_branch == \"${PREV_RELEASE_BRANCH}\"/" "$PREV_FILE"
          git add "$PREV_FILE"
          echo "   ✅ Updated: $PREV_FILE (main -> ${PREV_RELEASE_BRANCH})"
          PREV_RELEASE_UPDATED=true
        else
          echo "   ⚠️  Unexpected target_branch in $PREV_FILE: $CURRENT_TARGET" >&2
        fi
      else
        echo "   ⚠️  File not found: $PREV_FILE" >&2
      fi
    done
  done

  # Update GitHub workflow files
  echo ""
  echo "   Updating GitHub workflow files..."

  WORKFLOWS=(".github/workflows/auto_code_review.yml" ".github/workflows/e2e.yml")

  for workflow in "${WORKFLOWS[@]}"; do
    if [[ -f "$workflow" ]]; then
      # Check if already pointing to release branch
      if grep -q "branches:.*- main" "$workflow" 2>/dev/null; then
        # Update branches from "main" to the release branch
        sed "${SED_INPLACE[@]}" "s/- main/- ${PREV_RELEASE_BRANCH}/g" "$workflow"
        git add "$workflow"
        echo "   ✅ Updated $workflow (main -> ${PREV_RELEASE_BRANCH})"
        PREV_RELEASE_UPDATED=true
      else
        echo "   ✓ $workflow already points to correct branch"
      fi
    else
      echo "   ℹ️  File not found: $workflow"
    fi
  done

  # Commit changes if any
  if [[ "$PREV_RELEASE_UPDATED" = true ]]; then
    # Create a branch for PR to previous release
    PREV_PR_BRANCH="update-${PREV_RELEASE_BRANCH}-tekton-target"
    git checkout -b "$PREV_PR_BRANCH"

    git commit --signoff -m "Update ${PREV_TAG} pipelines and workflows to target ${PREV_RELEASE_BRANCH}

- Update .tekton/ target_branch from main to ${PREV_RELEASE_BRANCH}
- Update GitHub workflows (auto_code_review.yml, e2e.yml) to trigger on ${PREV_RELEASE_BRANCH}
- Ensures pipelines and CI/CD trigger for the correct release branch

ACM: ${PREV_RELEASE_BRANCH}, Global Hub: release-${PREV_GH_VERSION_SHORT}"

    echo "   ✅ Changes committed to $PREV_PR_BRANCH"

    # Check if PR already exists
    echo "   Checking for existing PR to ${PREV_RELEASE_BRANCH}..."
    PREV_PR_HEAD="${ORIGIN_USER}:${PREV_PR_BRANCH}"
    EXISTING_PREV_PR=$(gh pr list \
      --repo "${REPO_ORG}/${REPO_NAME}" \
      --head "${PREV_PR_HEAD}" \
      --base "${PREV_RELEASE_BRANCH}" \
      --state all \
      --json number,url,state \
      --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

    # Push PR branch to origin
    echo "   Pushing $PREV_PR_BRANCH to origin..."
    if git push -f origin "$PREV_PR_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin"

      # Check if PR exists
      if [[ -n "$EXISTING_PREV_PR" && "$EXISTING_PREV_PR" != "$NULL_PR_VALUE" ]]; then
        PREV_PR_STATE=$(echo "$EXISTING_PREV_PR" | cut -d'|' -f1)
        PREV_PR_URL=$(echo "$EXISTING_PREV_PR" | cut -d'|' -f2)
        echo "   ✅ PR already exists and updated (state: $PREV_PR_STATE): $PREV_PR_URL"
        PREV_PR_CREATED=true
      else
        # Create new PR to previous release branch
        echo "   Creating PR to ${REPO_ORG}:${PREV_RELEASE_BRANCH}..."
        PR_CREATE_OUTPUT=$(gh pr create \
          --repo "${REPO_ORG}/${REPO_NAME}" \
          --base "${PREV_RELEASE_BRANCH}" \
          --head "${PREV_PR_HEAD}" \
          --title "Fix ${PREV_TAG} tekton pipelines target_branch to ${PREV_RELEASE_BRANCH}" \
          --body "## Summary

Update pipeline target_branch for ${PREV_RELEASE_BRANCH}.

## Changes

- Update .tekton/ files for \`${PREV_TAG}\` to set \`target_branch=${PREV_RELEASE_BRANCH}\`
- Ensures pipelines trigger correctly for the release branch

## Release Info

- **ACM**: ${PREV_RELEASE_BRANCH}
- **Global Hub**: release-${PREV_GH_VERSION_SHORT}" 2>&1) || true

        # Check if PR was successfully created or already exists
        if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
          PREV_PR_URL="$PR_CREATE_OUTPUT"
          echo "   ✅ PR created for previous release: $PREV_PR_URL"
          PREV_PR_CREATED=true
        elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
          # PR already exists, extract URL from error message
          PREV_PR_URL="${BASH_REMATCH[1]}"
          echo "   ✅ PR already exists and updated: $PREV_PR_URL"
          PREV_PR_CREATED=true
        else
          echo "   ⚠️  Failed to create PR for previous release" >&2
          echo "   Reason: $PR_CREATE_OUTPUT"
          PREV_PR_CREATED=false
        fi
      fi
    else
      echo "   ⚠️  Failed to push branch for previous release PR" >&2
      PREV_PR_CREATED=false
    fi
  else
    echo "   ℹ️  No updates needed for previous release"
    PREV_PR_CREATED=false
  fi
fi

# Step 10: Update current release .tekton files to point to current release branch
echo ""
echo "📍 Step 10: Updating current release .tekton files..."

# Clean any uncommitted changes before proceeding
git reset --hard HEAD 2>/dev/null || true
git clean -fd 2>/dev/null || true

# Check if current release branch exists
CURRENT_RELEASE_EXISTS=false
if git ls-remote --heads "$UPSTREAM_REMOTE" "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  CURRENT_RELEASE_EXISTS=true
  echo "   ✓ Current release branch exists: $RELEASE_BRANCH"
else
  echo "   ⚠️  Current release branch not found: $RELEASE_BRANCH" >&2
  echo "   Skipping current release update"
fi

CURRENT_RELEASE_UPDATED=false
CURRENT_PR_CREATED=false

if [[ "$CURRENT_RELEASE_EXISTS" = true ]]; then
  # Checkout current release branch
  echo "   Checking out $RELEASE_BRANCH..."
  git fetch "$UPSTREAM_REMOTE" "$RELEASE_BRANCH" >/dev/null 2>&1
  git checkout -B "$RELEASE_BRANCH" "$UPSTREAM_REMOTE/$RELEASE_BRANCH"

  # Update the current version .tekton files to point to current release branch
  NEW_TAG="globalhub-${GH_VERSION_SHORT//./-}"

  echo "   Updating ${NEW_TAG} files to target_branch=${RELEASE_BRANCH}..."

  for component in agent manager operator; do
    for pipeline_type in pull-request push; do
      CURRENT_FILE=".tekton/multicluster-global-hub-${component}-${NEW_TAG}-${pipeline_type}.yaml"

      # Determine expected target_branch based on pipeline type
      if [[ "$pipeline_type" = "pull-request" ]]; then
        EXPECTED_TARGET="main"
      else
        # push pipelines should target the release branch
        EXPECTED_TARGET="$RELEASE_BRANCH"
      fi

      if [[ -f "$CURRENT_FILE" ]]; then
        # Check current target_branch
        CURRENT_TARGET=$(grep "$TARGET_BRANCH_GREP_PATTERN" "$CURRENT_FILE" | sed -E "$TARGET_BRANCH_EXTRACT_SED" || echo "")

        if [[ "$CURRENT_TARGET" = "$EXPECTED_TARGET" ]]; then
          echo "   ℹ️  Already correct: $CURRENT_FILE (target_branch=$EXPECTED_TARGET)"
        elif [[ -n "$CURRENT_TARGET" ]]; then
          # Update to expected target_branch
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$CURRENT_FILE"
          git add "$CURRENT_FILE"
          echo "   ✅ Updated: $CURRENT_FILE ($CURRENT_TARGET -> ${EXPECTED_TARGET})"
          CURRENT_RELEASE_UPDATED=true
        else
          echo "   ⚠️  Cannot find target_branch in $CURRENT_FILE" >&2
        fi
      else
        echo "   ⚠️  File not found: $CURRENT_FILE" >&2
      fi
    done
  done

  # Commit changes if any
  if [[ "$CURRENT_RELEASE_UPDATED" = true ]]; then
    # Create a branch for PR to current release
    CURRENT_PR_BRANCH="update-${RELEASE_BRANCH}-tekton-target"
    git checkout -b "$CURRENT_PR_BRANCH"

    git commit --signoff -m "Update ${NEW_TAG} pipelines to target ${RELEASE_BRANCH}

- Update .tekton/ target_branch to ${RELEASE_BRANCH}
- pull-request pipelines: target_branch=main
- push pipelines: target_branch=${RELEASE_BRANCH}
- Ensures pipelines trigger for the correct release branch

ACM: ${RELEASE_BRANCH}, Global Hub: release-${GH_VERSION_SHORT}"

    echo "   ✅ Changes committed to $CURRENT_PR_BRANCH"

    # Check if PR already exists
    echo "   Checking for existing PR to ${RELEASE_BRANCH}..."
    CURRENT_PR_HEAD="${ORIGIN_USER}:${CURRENT_PR_BRANCH}"
    EXISTING_CURRENT_PR=$(gh pr list \
      --repo "${REPO_ORG}/${REPO_NAME}" \
      --head "${CURRENT_PR_HEAD}" \
      --base "${RELEASE_BRANCH}" \
      --state all \
      --json number,url,state \
      --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

    # Push PR branch to origin
    echo "   Pushing $CURRENT_PR_BRANCH to origin..."
    if git push -f origin "$CURRENT_PR_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin"

      # Check if PR exists
      if [[ -n "$EXISTING_CURRENT_PR" && "$EXISTING_CURRENT_PR" != "$NULL_PR_VALUE" ]]; then
        CURRENT_PR_STATE=$(echo "$EXISTING_CURRENT_PR" | cut -d'|' -f1)
        CURRENT_PR_URL=$(echo "$EXISTING_CURRENT_PR" | cut -d'|' -f2)
        echo "   ✅ PR already exists and updated (state: $CURRENT_PR_STATE): $CURRENT_PR_URL"
        CURRENT_PR_CREATED=true
      else
        # Create new PR to current release branch
        echo "   Creating PR to ${REPO_ORG}:${RELEASE_BRANCH}..."
        PR_CREATE_OUTPUT=$(gh pr create \
          --repo "${REPO_ORG}/${REPO_NAME}" \
          --base "${RELEASE_BRANCH}" \
          --head "${CURRENT_PR_HEAD}" \
          --title "Fix ${NEW_TAG} tekton pipelines target_branch for ${RELEASE_BRANCH}" \
          --body "## Summary

Update pipeline target_branch for ${RELEASE_BRANCH}.

## Changes

- Update .tekton/ files for \`${NEW_TAG}\`:
  - \`*-pull-request.yaml\`: \`target_branch=main\`
  - \`*-push.yaml\`: \`target_branch=${RELEASE_BRANCH}\`
- Ensures pipelines trigger correctly for the release branch

## Release Info

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${GH_VERSION_SHORT}" 2>&1) || true

        # Check if PR was successfully created or already exists
        if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
          CURRENT_PR_URL="$PR_CREATE_OUTPUT"
          echo "   ✅ PR created for current release: $CURRENT_PR_URL"
          CURRENT_PR_CREATED=true
        elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
          # PR already exists, extract URL from error message
          CURRENT_PR_URL="${BASH_REMATCH[1]}"
          echo "   ✅ PR already exists and updated: $CURRENT_PR_URL"
          CURRENT_PR_CREATED=true
        else
          echo "   ⚠️  Failed to create PR for current release" >&2
          echo "   Reason: $PR_CREATE_OUTPUT"
          CURRENT_PR_CREATED=false
        fi
      fi
    else
      echo "   ⚠️  Failed to push branch for current release PR" >&2
      CURRENT_PR_CREATED=false
    fi
  else
    echo "   ℹ️  No updates needed for current release"
    CURRENT_PR_CREATED=false
  fi
fi

# Summary
echo ""
echo "$SEPARATOR_LINE"
echo "📊 WORKFLOW SUMMARY"
echo "$SEPARATOR_LINE"
echo "Release: $RELEASE_BRANCH / release-${GH_VERSION_SHORT}"
echo ""

# Count tasks
COMPLETED=0
FAILED=0

echo "✅ COMPLETED TASKS:"

if [[ "$TEKTON_UPDATED" = true || "$CONTAINERFILE_UPDATED" = true ]]; then
  echo "  ✓ Updated main branch configurations"
  COMPLETED=$((COMPLETED + 1))
fi

if [[ "$RELEASE_BRANCH_PUSHED" = true || "$RELEASE_BRANCH_EXISTS" = true ]]; then
  if [[ "$RELEASE_BRANCH_EXISTS" = true ]]; then
    echo "  ✓ Release branch: $RELEASE_BRANCH (already existed)"
  else
    echo "  ✓ Release branch: $RELEASE_BRANCH (created and pushed)"
  fi
  COMPLETED=$((COMPLETED + 1))
fi

if [[ "$MAIN_PR_CREATED" = true ]]; then
  echo "  ✓ PR to main: ${MAIN_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

if [[ "$PREV_RELEASE_UPDATED" = true && "$PREV_PR_CREATED" = true ]]; then
  echo "  ✓ PR to $PREV_RELEASE_BRANCH: ${PREV_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

if [[ "$CURRENT_RELEASE_UPDATED" = true && "$CURRENT_PR_CREATED" = true ]]; then
  echo "  ✓ PR to $RELEASE_BRANCH: ${CURRENT_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

# Show any issues
SHOW_ISSUES=false
if [[ "$TEKTON_UPDATED" = false || "$CONTAINERFILE_UPDATED" = false || \
   "$MAIN_PR_CREATED" = false || "$RELEASE_BRANCH_PUSHED" = false ]]; then
  SHOW_ISSUES=true
fi

if [[ "$SHOW_ISSUES" = true ]]; then
  echo ""
  echo "⚠️  ISSUES / WARNINGS:" >&2

  if [[ "$TEKTON_UPDATED" = false ]]; then
    echo "  ⚠ .tekton/ files not created"
    FAILED=$((FAILED + 1))
  fi

  if [[ "$CONTAINERFILE_UPDATED" = false ]]; then
    echo "  ⚠ Containerfile labels not updated"
    FAILED=$((FAILED + 1))
  fi

  if [[ "$MAIN_PR_CREATED" = false && "$MAIN_CHANGES_COMMITTED" = true ]]; then
    echo "  ⚠ PR to main not created (manual creation needed)"
    FAILED=$((FAILED + 1))
  fi

  if [[ "$RELEASE_BRANCH_PUSHED" = false && "$RELEASE_BRANCH_EXISTS" = false ]]; then
    echo "  ⚠ Release branch not pushed (manual push needed)"
    FAILED=$((FAILED + 1))
  fi

  if [[ "$PREV_RELEASE_UPDATED" = false && "$PREV_RELEASE_EXISTS" = true ]]; then
    echo "  ℹ️  Previous release not updated (may already be correct)"
  fi
fi

echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"

PR_COUNT=0
if [[ "$MAIN_PR_CREATED" = true ]]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${MAIN_PR_URL}"
fi

if [[ "$PREV_PR_CREATED" = true ]]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${PREV_PR_URL}"
fi

if [[ "$CURRENT_PR_CREATED" = true ]]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${CURRENT_PR_URL}"
fi

echo ""
echo "After merge: Verify Konflux pipelines and builds"

echo ""
echo "$SEPARATOR_LINE"
if [[ $FAILED -eq 0 ]]; then
  echo "✅ SUCCESS ($COMPLETED tasks completed)"
else
  echo "⚠️  COMPLETED WITH WARNINGS ($COMPLETED completed, $FAILED warnings)" >&2
fi
echo "$SEPARATOR_LINE"
