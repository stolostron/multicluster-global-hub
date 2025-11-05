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

# Detect latest release from multicluster-global-hub configs
echo "   🔍 Detecting previous release from existing configs..."
LATEST_RELEASE=$(find ci-operator/config/stolostron/multicluster-global-hub/ -name 'stolostron-multicluster-global-hub-release-*.yaml' 2>/dev/null | \
  sed 's|.*/stolostron-multicluster-global-hub-||' | \
  sed 's|\.yaml||' | \
  sort -V | tail -1)

if [[ -z "$LATEST_RELEASE" ]]; then
  echo "   ❌ Error: Could not detect previous release from openshift/release configs" >&2
  exit 1
fi

echo "   Previous release detected: $LATEST_RELEASE"

# Calculate version strings for file updates
PREV_VERSION="${LATEST_RELEASE#release-}"
VERSION_SHORT="${ACM_VERSION/./}"
PREV_VERSION_SHORT="${PREV_VERSION/./}"

echo "   Previous: $LATEST_RELEASE"
echo "   Current:  $RELEASE_BRANCH"

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
if [[ -f "$NEW_CONFIG" ]]; then
  echo "   ℹ️  Configuration file already exists: $NEW_CONFIG"
  # Check if it's already been updated with the correct version
  if grep -q "branch: ${RELEASE_BRANCH}" "$NEW_CONFIG" && \
     grep -q "IMAGE_TAG: ${GH_VERSION}" "$NEW_CONFIG"; then
    echo "   ✓ Configuration already up to date"
  else
    echo "   ⚠️  File exists but needs updates, applying changes..." >&2
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
    # Don't exit, continue to summary to show PR link
  else
    echo "   ⚠️  No changes to commit and no PR exists" >&2
    # Continue to summary
  fi
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
- Create ${RELEASE_BRANCH} pipeline configuration based on ${LATEST_RELEASE}
- Update image-mirror job prefixes to release-${VERSION_SHORT}
- IMAGE_TAG is ${GH_VERSION}
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
2. **Create ${RELEASE_BRANCH} pipeline configuration**: Based on ${LATEST_RELEASE}
3. **Auto-generate job configurations**: Using \`make update\`

## Release Info

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${GH_VERSION_SHORT}
- **Job prefix**: \`release-${VERSION_SHORT}\`" \
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
