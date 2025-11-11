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
if [[ -z "$RELEASE_BRANCH" || -z "$ACM_VERSION" || -z "$GH_VERSION" || -z "$OPENSHIFT_RELEASE_PATH" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
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

# Constants for repeated patterns
readonly NULL_PR_VALUE='null|null'
readonly SEPARATOR_LINE='================================================'

echo "🚀 OpenShift Release Configuration"
echo "$SEPARATOR_LINE"
echo "   Release: $RELEASE_BRANCH / release-${GH_VERSION_SHORT}"
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
  echo "   ⚠️  Fork not found. Please fork https://github.com/openshift/release to your account first." >&2
  exit 1
fi

# Clone or use existing
if [[ -d "$OPENSHIFT_RELEASE_PATH" ]]; then
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
    echo "   ⚠️  Failed to update, continuing with existing state" >&2
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

# Calculate previous release version (ACM_VERSION - 0.01)
# Example: 2.16 -> 2.15, 2.17 -> 2.16
PREV_ACM_VERSION=$(echo "$ACM_VERSION" | awk '{printf "%.2f", $1 - 0.01}')
PREV_RELEASE_BRANCH="release-${PREV_ACM_VERSION}"

echo "   🔍 Calculating version mappings..."
echo "   Previous ACM:    $PREV_RELEASE_BRANCH"
echo "   Current ACM:     $RELEASE_BRANCH"

# Calculate Global Hub versions
# ACM 2.15 -> Global Hub 1.6, ACM 2.16 -> Global Hub 1.7
# Formula: GH_MINOR = ACM_MINOR - 9
PREV_GH_MINOR=$(echo "$PREV_ACM_VERSION" | awk -F. '{print $2 - 9}')
PREV_GH_VERSION="v1.${PREV_GH_MINOR}.0"

echo "   Previous GH:     $PREV_GH_VERSION (release-1.${PREV_GH_MINOR})"
echo "   Current GH:      $GH_VERSION (release-${GH_VERSION_SHORT})"

# Calculate image mirror task prefixes (without dots or dashes)
# Example: release-214, release-216
VERSION_SHORT="${ACM_VERSION/./}"
PREV_VERSION_SHORT="${PREV_ACM_VERSION/./}"
IMAGE_PREFIX="release-${VERSION_SHORT}"
PREV_IMAGE_PREFIX="release-${PREV_VERSION_SHORT}"

echo "   Previous prefix: $PREV_IMAGE_PREFIX"
echo "   Current prefix:  $IMAGE_PREFIX"

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
PREV_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${PREV_RELEASE_BRANCH}.yaml"
NEW_CONFIG="ci-operator/config/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}.yaml"

# Check if previous release config exists
echo "   Checking for previous release configuration..."
if [[ ! -f "$PREV_CONFIG" ]]; then
  echo "" >&2
  echo "   ❌ ERROR: Previous release config not found!" >&2
  echo "" >&2
  echo "   Expected file: $PREV_CONFIG" >&2
  echo "   Current release: $RELEASE_BRANCH (Global Hub: release-${GH_VERSION_SHORT})" >&2
  echo "   Previous release: $PREV_RELEASE_BRANCH (Global Hub: release-1.${PREV_GH_MINOR})" >&2
  echo "" >&2
  echo "   ⚠️  Please verify:" >&2
  echo "   1. Is RELEASE_BRANCH=$RELEASE_BRANCH correct?" >&2
  echo "   2. Has the previous release ($PREV_RELEASE_BRANCH) been created?" >&2
  echo "   3. Does $PREV_CONFIG exist in the repository?" >&2
  echo "" >&2
  echo "   You must create the previous release configuration before creating $RELEASE_BRANCH" >&2
  echo "" >&2
  exit 1
fi

echo "   ✅ Found previous config: $PREV_CONFIG"

# Update main branch configuration
echo "   Updating main branch configuration..."
echo "      Applying version replacements to main config..."

# Replace all occurrences of previous release branch pattern
sed "${SED_INPLACE[@]}" "s/${PREV_RELEASE_BRANCH}/${RELEASE_BRANCH}/g" "$MAIN_CONFIG"

# Replace all occurrences of previous ACM version (in quotes or standalone)
sed "${SED_INPLACE[@]}" "s/\"${PREV_ACM_VERSION}\"/\"${ACM_VERSION}\"/g" "$MAIN_CONFIG"
sed "${SED_INPLACE[@]}" "s/${PREV_ACM_VERSION}/${ACM_VERSION}/g" "$MAIN_CONFIG"

# Replace image mirror task prefix if present
sed "${SED_INPLACE[@]}" "s/${PREV_IMAGE_PREFIX}/${IMAGE_PREFIX}/g" "$MAIN_CONFIG"

# Replace Global Hub version if present
sed "${SED_INPLACE[@]}" "s/${PREV_GH_VERSION}/${GH_VERSION}/g" "$MAIN_CONFIG"

echo "      ✓ Applied: ${PREV_RELEASE_BRANCH} -> ${RELEASE_BRANCH}"
echo "      ✓ Applied: ${PREV_ACM_VERSION} -> ${ACM_VERSION}"
echo "      ✓ Applied: ${PREV_IMAGE_PREFIX} -> ${IMAGE_PREFIX} (if present)"
echo "      ✓ Applied: ${PREV_GH_VERSION} -> ${GH_VERSION} (if present)"
echo "   ✅ Updated $MAIN_CONFIG"

# Create or update new release configuration
echo "   Creating $RELEASE_BRANCH pipeline configuration..."

# If target file already exists, remove it first to ensure clean copy
if [[ -f "$NEW_CONFIG" ]]; then
  echo "   ℹ️  Target file already exists, removing for clean copy..."
  rm -f "$NEW_CONFIG"
  echo "   ✓ Removed existing file"
fi

# Copy from previous release configuration
echo "   Copying from previous release config: $PREV_CONFIG"
cp "$PREV_CONFIG" "$NEW_CONFIG"
echo "   ✓ Copied to: $NEW_CONFIG"

# Apply all version replacements
echo "   Applying version replacements..."

# 1. Replace ACM release branch (e.g., release-2.15 -> release-2.16)
sed "${SED_INPLACE[@]}" "s/${PREV_RELEASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_CONFIG"
echo "      ✓ ${PREV_RELEASE_BRANCH} -> ${RELEASE_BRANCH}"

# 2. Replace ACM version string (e.g., "2.15" -> "2.16")
sed "${SED_INPLACE[@]}" "s/\"${PREV_ACM_VERSION}\"/\"${ACM_VERSION}\"/g" "$NEW_CONFIG"
sed "${SED_INPLACE[@]}" "s/${PREV_ACM_VERSION}/${ACM_VERSION}/g" "$NEW_CONFIG"
echo "      ✓ ${PREV_ACM_VERSION} -> ${ACM_VERSION}"

# 3. Replace image mirror task prefix pattern (e.g., release-214|release-215|release-2*- -> release-216-)
# First, replace any pattern like release-2XX- to the new prefix
sed "${SED_INPLACE[@]}" "s/release-2[0-9][0-9]-/${IMAGE_PREFIX}-/g" "$NEW_CONFIG"
# Also handle the specific previous prefix
sed "${SED_INPLACE[@]}" "s/${PREV_IMAGE_PREFIX}/${IMAGE_PREFIX}/g" "$NEW_CONFIG"
echo "      ✓ release-2XX- -> ${IMAGE_PREFIX}-"

# 4. Replace Global Hub version tag (e.g., v1.6.0 -> v1.7.0)
sed "${SED_INPLACE[@]}" "s/${PREV_GH_VERSION}/${GH_VERSION}/g" "$NEW_CONFIG"
echo "      ✓ ${PREV_GH_VERSION} -> ${GH_VERSION}"

echo "   ✅ Created and updated $NEW_CONFIG"

# Verify the replacements were successful
echo "   Verifying replacements..."
VERIFY_ERRORS=0

if ! grep -q "branch: ${RELEASE_BRANCH}" "$NEW_CONFIG"; then
  echo "   ⚠️  Warning: 'branch: ${RELEASE_BRANCH}' not found in config" >&2
  VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

if ! grep -q "name: \"${ACM_VERSION}\"" "$NEW_CONFIG"; then
  echo "   ⚠️  Warning: 'name: \"${ACM_VERSION}\"' not found in config" >&2
  VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

if ! grep -q "${IMAGE_PREFIX}" "$NEW_CONFIG"; then
  echo "   ⚠️  Warning: '${IMAGE_PREFIX}' prefix not found in config" >&2
  VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

if ! grep -q "${GH_VERSION}" "$NEW_CONFIG"; then
  echo "   ⚠️  Warning: '${GH_VERSION}' not found in config" >&2
  VERIFY_ERRORS=$((VERIFY_ERRORS + 1))
fi

if [[ $VERIFY_ERRORS -eq 0 ]]; then
  echo "   ✓ All replacements verified successfully"
else
  echo "   ⚠️  Some replacements may not have been applied correctly" >&2
  echo "   Please review the generated file: $NEW_CONFIG" >&2
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
    echo "   ❌ Error: Podman is installed but no machine is running" >&2
    echo ""
    echo "   Please start your podman machine:"
    echo "      podman machine start"
    echo ""
    exit 1
  fi
else
  echo "   ❌ Error: No container engine found!" >&2
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
if [[ "$CONTAINER_ENGINE" = "docker" ]]; then
  if timeout 120 bash -c "CONTAINER_ENGINE=docker make update" 2>/dev/null; then
    MAKE_UPDATE_SUCCESS=true
    echo "   ✅ Job configurations generated"
  else
    echo "   ⚠️  make update timed out or failed (skipping)" >&2
    echo "   ℹ️  You may need to run 'make update' manually in the PR"
  fi
else
  if timeout 120 make update 2>/dev/null; then
    MAKE_UPDATE_SUCCESS=true
    echo "   ✅ Job configurations generated"
  else
    echo "   ⚠️  make update timed out or failed (skipping)" >&2
    echo "   ℹ️  You may need to run 'make update' manually in the PR"
  fi
fi

# Step 5: Commit and create/update PR
echo ""
echo "📍 Step 5: Committing changes and creating/updating PR..."

# Check for existing PR
echo "   Checking for existing PR..."
PR_HEAD="${GITHUB_USER}:${BRANCH_NAME}"
EXISTING_PR=$(gh pr list \
  --repo "openshift/release" \
  --head "${PR_HEAD}" \
  --base master \
  --state all \
  --json number,url,state \
  --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

# Initialize PR tracking variables
PR_CREATED=false
PR_URL=""

# Check if there are changes to commit
CHANGES_EXIST=false
if ! git diff --quiet || ! git diff --cached --quiet; then
  CHANGES_EXIST=true
fi

# If no changes and PR already exists, we're done
if [[ "$CHANGES_EXIST" = false ]]; then
  if [[ -n "$EXISTING_PR" && "$EXISTING_PR" != "$NULL_PR_VALUE" ]]; then
    PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
    PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
    PR_CREATED=true
    echo "   ℹ️  No changes needed, PR already up to date (state: $PR_STATE)"
    echo "   ✓ PR: ${PR_URL}"
  else
    echo "   ⚠️  No changes to commit and no PR exists" >&2
  fi
else
  # We have changes to commit
  echo "   Changes detected, preparing commit..."

  # Add files
  git add "$MAIN_CONFIG"
  git add "$NEW_CONFIG"

  # Add job files if they exist (may not exist if make update failed)
  if [[ "$MAKE_UPDATE_SUCCESS" = true ]]; then
    git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}-presubmits.yaml" 2>/dev/null || true
    git add "ci-operator/jobs/stolostron/multicluster-global-hub/stolostron-multicluster-global-hub-${RELEASE_BRANCH}-postsubmits.yaml" 2>/dev/null || true
    COMMIT_NOTE="- Auto-generate presubmits and postsubmits using make update"
  else
    COMMIT_NOTE="- Job files need to be generated manually (make update failed/timed out)"
  fi

  # Commit
  git commit --signoff -m "Add ${RELEASE_BRANCH} configuration for multicluster-global-hub

- Update main branch to promote to ${ACM_VERSION} and fast-forward to ${RELEASE_BRANCH}
- Create ${RELEASE_BRANCH} pipeline configuration based on ${PREV_RELEASE_BRANCH}
- Update image-mirror job prefixes from ${PREV_IMAGE_PREFIX} to ${IMAGE_PREFIX}
- IMAGE_TAG updated from ${PREV_GH_VERSION} to ${GH_VERSION}
${COMMIT_NOTE}

ACM: ${RELEASE_BRANCH}, Global Hub: release-${GH_VERSION_SHORT}"

  echo "   ✅ Changes committed"

  # Push to fork (force push is safe for updating PRs)
  echo "   Pushing to origin/${BRANCH_NAME}..."
  if git push -f origin "$BRANCH_NAME" 2>&1; then
    echo "   ✅ Branch pushed to origin"
    PUSH_SUCCESS=true
  else
    echo "   ⚠️  Failed to push branch to origin" >&2
    PUSH_SUCCESS=false
    # Don't exit, show summary with error status
  fi

  # Check if PR exists and create/update accordingly (only if push succeeded)
  if [[ "$PUSH_SUCCESS" = true ]]; then
    if [[ -n "$EXISTING_PR" && "$EXISTING_PR" != "$NULL_PR_VALUE" ]]; then
      PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
      PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
      echo "   ✅ PR already exists and updated (state: $PR_STATE): $PR_URL"
      PR_CREATED=true
    else
      # Create new PR
      echo "   Creating PR to openshift/release:master..."
      PR_CREATE_OUTPUT=$(gh pr create --base master --head "$PR_HEAD" \
        --title "Add ${RELEASE_BRANCH} configuration for multicluster-global-hub" \
        --body "This PR adds ${RELEASE_BRANCH} configuration for the multicluster-global-hub project.

## Changes

1. **Update main branch configuration**: Promote to ${ACM_VERSION}, fast-forward to ${RELEASE_BRANCH}
2. **Create ${RELEASE_BRANCH} pipeline configuration**: Based on ${PREV_RELEASE_BRANCH}
3. **Version replacements applied**:
   - Branch: \`${PREV_RELEASE_BRANCH}\` → \`${RELEASE_BRANCH}\`
   - Version: \`${PREV_ACM_VERSION}\` → \`${ACM_VERSION}\`
   - Image prefix: \`${PREV_IMAGE_PREFIX}\` → \`${IMAGE_PREFIX}\`
   - Global Hub: \`${PREV_GH_VERSION}\` → \`${GH_VERSION}\`
4. **Auto-generate job configurations**: Using \`make update\`

## Release Info

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${GH_VERSION_SHORT} (${GH_VERSION})
- **Previous**: ${PREV_RELEASE_BRANCH} / ${PREV_GH_VERSION}
- **Job prefix**: \`${IMAGE_PREFIX}\`" \
        --repo openshift/release 2>&1) || true

      # Check if PR was successfully created or already exists
      if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
        PR_URL="$PR_CREATE_OUTPUT"
        echo "   ✅ PR created: $PR_URL"
        PR_CREATED=true
      elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
        # PR already exists, extract URL from error message
        PR_URL="${BASH_REMATCH[1]}"
        echo "   ✅ PR already exists and updated: $PR_URL"
        PR_CREATED=true
      else
        echo "   ⚠️  Failed to create PR automatically" >&2
        echo "   Reason: $PR_CREATE_OUTPUT"
        PR_CREATED=false
      fi
    fi
  fi
fi

# Summary
echo ""
echo "$SEPARATOR_LINE"
echo "📊 WORKFLOW SUMMARY"
echo "$SEPARATOR_LINE"
echo "Release: $RELEASE_BRANCH / release-${GH_VERSION_SHORT}"
echo ""
echo "✅ COMPLETED TASKS:"
if [[ "$CHANGES_EXIST" = true ]]; then
  echo "  ✓ Updated main branch config"
  echo "  ✓ Created ${RELEASE_BRANCH} config"
  if [[ "$MAKE_UPDATE_SUCCESS" = true ]]; then
    echo "  ✓ Generated job configurations"
  else
    echo "  ⚠️  Job generation skipped (timeout/error)" >&2
  fi
fi
if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
  echo "  ✓ PR: ${PR_URL}"
fi
echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"
if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
  echo "1. Review and merge: ${PR_URL}"
fi
if [[ "$MAKE_UPDATE_SUCCESS" != true ]]; then
  echo ""
  echo "⚠️  make update failed - Manual steps required:" >&2
  echo "   cd $OPENSHIFT_RELEASE_PATH && git checkout $BRANCH_NAME"
  echo "   make update && git add ci-operator/jobs/"
  echo "   git commit --amend --no-edit && git push -f"
fi
echo ""
echo "After merge: Verify CI jobs in openshift/release"
echo ""
echo "$SEPARATOR_LINE"
if [[ "$MAKE_UPDATE_SUCCESS" = true && "$PR_CREATED" = true ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH WARNINGS" >&2
fi
echo "$SEPARATOR_LINE"
