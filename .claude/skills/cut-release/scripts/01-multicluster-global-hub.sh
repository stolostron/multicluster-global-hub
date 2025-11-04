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
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$GH_VERSION_SHORT" || -z "$ACM_VERSION" || -z "$CUT_MODE" || -z "$GITHUB_USER" ]]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GH_VERSION_SHORT, ACM_VERSION, CUT_MODE, GITHUB_USER"
  exit 1
fi

# Always use temporary directory for clean clone
WORK_DIR="${WORK_DIR:-/tmp/multicluster-global-hub-release}"

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS requires -i with empty string
  SED_INPLACE=(-i "")
else
  # Linux uses -i without argument
  SED_INPLACE=(-i)
fi

echo "🚀 Multicluster Global Hub Release Workflow"
echo "================================================"
echo "   Mode: $([ "$CUT_MODE" = true ] && echo "CUT (create branches)" || echo "UPDATE (PR only)")"
echo ""
echo "Workflow:"
echo "  1. Update main branch with new .tekton files (target_branch=main)"
if [ "$CUT_MODE" = "true" ]; then
  echo "  2. Create release branch from updated main (CUT MODE)"
else
  echo "  2. Skip release branch creation (UPDATE MODE)"
fi
echo "  3. Create PR to upstream main with new release configurations"
echo "  4. Update previous release .tekton files (target_branch=previous_release_branch)"
echo "  5. Update current release .tekton files (target_branch=current_release_branch)"
echo "================================================"

# Step 1: Clone repository and setup remotes
echo ""
echo "📍 Step 1: Cloning repository..."

# Remove existing work directory for clean clone
if [ -d "$WORK_DIR" ]; then
  echo "   Removing existing work directory..."
  rm -rf "$WORK_DIR"
fi

# Clone from upstream with shallow clone for speed
echo "   Cloning ${REPO_ORG}/${REPO_NAME} (--depth=1 for faster clone)..."
git clone --depth=1 --single-branch --branch main --progress "https://github.com/${REPO_ORG}/${REPO_NAME}.git" "$WORK_DIR" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
cd "$WORK_DIR"

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
echo "   ✅ Repository ready at $WORK_DIR"

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

if [ ! -d ".tekton" ]; then
  echo "   ⚠️  .tekton/ directory not found in current repository"
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
      if [ "$pipeline_type" = "pull-request" ]; then
        EXPECTED_TARGET="main"
      else
        # push pipelines should target the release branch
        EXPECTED_TARGET="$RELEASE_BRANCH"
      fi

      # Check if new file already exists
      if [ -f "$NEW_FILE" ]; then
        echo "   ℹ️  Already exists: $NEW_FILE"

        # Check current target_branch
        CURRENT_TARGET=$(grep 'target_branch ==' "$NEW_FILE" | sed -E 's/.*target_branch == "([^"]+)".*/\1/' || echo "")

        if [ "$CURRENT_TARGET" = "$EXPECTED_TARGET" ]; then
          echo "   ✓ Content verified: target_branch=$EXPECTED_TARGET"
          TEKTON_UPDATED=true
        elif [ -n "$CURRENT_TARGET" ]; then
          # Update target_branch to expected value
          echo "   ⚠️  Updating target_branch: $CURRENT_TARGET -> $EXPECTED_TARGET"
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$NEW_FILE"
          git add "$NEW_FILE"
          echo "   ✅ Updated: $NEW_FILE (target_branch=$EXPECTED_TARGET)"
          TEKTON_UPDATED=true
        else
          echo "   ⚠️  Cannot find target_branch in $NEW_FILE"
        fi
        continue
      fi

      # Old file exists, copy to new file
      if [ -f "$OLD_FILE" ]; then
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
        CURRENT_TARGET=$(grep 'target_branch ==' "$NEW_FILE" | sed -E 's/.*target_branch == "([^"]+)".*/\1/' || echo "")
        if [ -n "$CURRENT_TARGET" ] && [ "$CURRENT_TARGET" != "$EXPECTED_TARGET" ]; then
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$NEW_FILE"
        fi

        # Verify target_branch is set correctly
        if ! grep -q "target_branch == \"${EXPECTED_TARGET}\"" "$NEW_FILE"; then
          echo "   ⚠️  Warning: target_branch not set to $EXPECTED_TARGET in $NEW_FILE"
        fi

        git add "$NEW_FILE"
        echo "   ✅ Updated content in $NEW_FILE (target_branch=$EXPECTED_TARGET)"
        TEKTON_UPDATED=true
      else
        echo "   ⚠️  Source file not found: $OLD_FILE"
      fi
    done
  done

  if [ "$TEKTON_UPDATED" = true ]; then
    echo "   ✅ Tekton files updated"
  else
    echo "   ⚠️  No updates needed"
  fi
fi

# Step 5: Update Containerfile.* version labels (idempotent)
echo ""
echo "📍 Step 5: Updating Containerfile.* version labels..."

CONTAINERFILE_UPDATED=false

# Update specific Containerfile locations
for component in agent manager operator; do
  CONTAINERFILE="${component}/Containerfile.${component}"

  if [ -f "$CONTAINERFILE" ]; then
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
      if [ -n "$VERSION_LINE" ]; then
        echo "   ⚠️  Found: $VERSION_LINE"
        echo "   ⚠️  Expected: release-${PREV_GH_VERSION_SHORT} or release-${GH_VERSION_SHORT}"
        echo "   ⚠️  Manual update may be needed"
      else
        echo "   ℹ️  No version label found in $CONTAINERFILE"
      fi
    fi
  else
    echo "   ⚠️  File not found: $CONTAINERFILE"
  fi
done

if [ "$CONTAINERFILE_UPDATED" = true ]; then
  echo "   ✅ Containerfile labels updated"
else
  echo "   ⚠️  No updates needed"
fi

# Step 6: Commit changes for main branch PR
echo ""
echo "📍 Step 6: Committing changes for main branch..."

# Check for staged and unstaged changes
if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  MAIN_CHANGES_COMMITTED=false
else
  # Stage all changes
  git add .tekton/ 2>/dev/null || true
  git add -- */Containerfile.* 2>/dev/null || true

  git commit --signoff -m "Add ${RELEASE_BRANCH} pipeline configurations

- Add new .tekton/ pipelines for ${NEW_TAG} (target_branch=main)
- Update Containerfile version labels to release-${GH_VERSION_SHORT}
- Keep existing ${PREV_TAG} pipelines for backward compatibility

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
if [ "$RELEASE_BRANCH_EXISTS" = true ]; then
  echo "   ℹ️  Release branch already exists, skipping creation"
  echo "   Note: Release branch exists on upstream, no merge needed"
  # Just checkout to continue with other steps
  git checkout -B "$RELEASE_BRANCH" "$UPSTREAM_REMOTE/$RELEASE_BRANCH"
else
  if [ "$CUT_MODE" != "true" ]; then
    echo "   ℹ️  Release branch does not exist - skipping (UPDATE mode)"
    echo "   Note: Run with CUT_MODE=true to create the release branch"
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

# Push PR branch to origin (your fork) - always push if there are changes
if [ "$MAIN_CHANGES_COMMITTED" = true ]; then
  git checkout "$MAIN_PR_BRANCH"

  echo "   Pushing $MAIN_PR_BRANCH to origin (${ORIGIN_USER})..."

  # Push to origin (force push is safe for your own fork)
  if git push -f origin "$MAIN_PR_BRANCH" 2>&1; then
    echo "   ✅ Branch pushed to origin"
    PUSH_SUCCESS=true
  else
    echo "   ⚠️  Failed to push branch to origin"
    PUSH_SUCCESS=false
  fi

  if [ "$PUSH_SUCCESS" = true ]; then
    # Check if PR exists
    if [ -n "$EXISTING_MAIN_PR" ] && [ "$EXISTING_MAIN_PR" != "null|null" ]; then
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
        --title "Add ${RELEASE_BRANCH} tekton pipelines and update Containerfile versions" \
        --body "## Summary

Add new pipeline configurations for ${RELEASE_BRANCH} to the main branch.

## Changes

- Add new .tekton/ pipeline files for \`${NEW_TAG}\`:
  - \`*-pull-request.yaml\`: \`target_branch=main\`
  - \`*-push.yaml\`: \`target_branch=${RELEASE_BRANCH}\`
- Update Containerfile version labels to \`release-${GH_VERSION_SHORT}\`
- Keep existing \`${PREV_TAG}\` pipelines for backward compatibility

## Release Info

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: release-${GH_VERSION_SHORT}

## Note

This PR adds the new release pipeline configurations to main branch while preserving the existing pipelines." 2>&1) || true

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
        echo "   ⚠️  Failed to create PR automatically"
        echo "   Reason: $PR_CREATE_OUTPUT"
        echo "   ℹ️  You can create the PR manually at:"
        echo "      https://github.com/${REPO_ORG}/${REPO_NAME}/compare/main...${PR_HEAD}"
        MAIN_PR_CREATED=false
      fi
    fi
  else
    echo "   ⚠️  Skipping PR creation/update due to push failure"
    MAIN_PR_CREATED=false
  fi
else
  echo "   ℹ️  No changes to push"
  # Still check if PR exists
  if [ -n "$EXISTING_MAIN_PR" ] && [ "$EXISTING_MAIN_PR" != "null|null" ]; then
    MAIN_PR_STATE=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f1)
    MAIN_PR_URL=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f2)
    echo "   ℹ️  PR already exists (state: $MAIN_PR_STATE): $MAIN_PR_URL"
    MAIN_PR_CREATED=true
  else
    MAIN_PR_CREATED=false
  fi
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
  echo "   ⚠️  Previous release branch not found: $PREV_RELEASE_BRANCH"
  echo "   Skipping previous release update"
fi

PREV_RELEASE_UPDATED=false

if [ "$PREV_RELEASE_EXISTS" = true ]; then
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

      if [ -f "$PREV_FILE" ]; then
        # Check current target_branch
        CURRENT_TARGET=$(grep 'target_branch ==' "$PREV_FILE" | sed -E 's/.*target_branch == "([^"]+)".*/\1/' || echo "")

        if [ "$CURRENT_TARGET" = "$PREV_RELEASE_BRANCH" ]; then
          echo "   ℹ️  Already correct: $PREV_FILE (target_branch=$PREV_RELEASE_BRANCH)"
        elif [ "$CURRENT_TARGET" = "main" ]; then
          # Update from main to release branch
          sed "${SED_INPLACE[@]}" "s/target_branch == \"main\"/target_branch == \"${PREV_RELEASE_BRANCH}\"/" "$PREV_FILE"
          git add "$PREV_FILE"
          echo "   ✅ Updated: $PREV_FILE (main -> ${PREV_RELEASE_BRANCH})"
          PREV_RELEASE_UPDATED=true
        else
          echo "   ⚠️  Unexpected target_branch in $PREV_FILE: $CURRENT_TARGET"
        fi
      else
        echo "   ⚠️  File not found: $PREV_FILE"
      fi
    done
  done

  # Commit changes if any
  if [ "$PREV_RELEASE_UPDATED" = true ]; then
    # Create a branch for PR to previous release
    PREV_PR_BRANCH="update-${PREV_RELEASE_BRANCH}-tekton-target"
    git checkout -b "$PREV_PR_BRANCH"

    git commit --signoff -m "Update ${PREV_TAG} pipelines to target ${PREV_RELEASE_BRANCH}

- Update .tekton/ target_branch from main to ${PREV_RELEASE_BRANCH}
- Ensures pipelines trigger for the correct release branch

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
      if [ -n "$EXISTING_PREV_PR" ] && [ "$EXISTING_PREV_PR" != "null|null" ]; then
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
          echo "   ⚠️  Failed to create PR for previous release"
          echo "   Reason: $PR_CREATE_OUTPUT"
          PREV_PR_CREATED=false
        fi
      fi
    else
      echo "   ⚠️  Failed to push branch for previous release PR"
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
  echo "   ⚠️  Current release branch not found: $RELEASE_BRANCH"
  echo "   Skipping current release update"
fi

CURRENT_RELEASE_UPDATED=false
CURRENT_PR_CREATED=false

if [ "$CURRENT_RELEASE_EXISTS" = true ]; then
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
      if [ "$pipeline_type" = "pull-request" ]; then
        EXPECTED_TARGET="main"
      else
        # push pipelines should target the release branch
        EXPECTED_TARGET="$RELEASE_BRANCH"
      fi

      if [ -f "$CURRENT_FILE" ]; then
        # Check current target_branch
        CURRENT_TARGET=$(grep 'target_branch ==' "$CURRENT_FILE" | sed -E 's/.*target_branch == "([^"]+)".*/\1/' || echo "")

        if [ "$CURRENT_TARGET" = "$EXPECTED_TARGET" ]; then
          echo "   ℹ️  Already correct: $CURRENT_FILE (target_branch=$EXPECTED_TARGET)"
        elif [ -n "$CURRENT_TARGET" ]; then
          # Update to expected target_branch
          sed "${SED_INPLACE[@]}" "s/target_branch == \"${CURRENT_TARGET}\"/target_branch == \"${EXPECTED_TARGET}\"/" "$CURRENT_FILE"
          git add "$CURRENT_FILE"
          echo "   ✅ Updated: $CURRENT_FILE ($CURRENT_TARGET -> ${EXPECTED_TARGET})"
          CURRENT_RELEASE_UPDATED=true
        else
          echo "   ⚠️  Cannot find target_branch in $CURRENT_FILE"
        fi
      else
        echo "   ⚠️  File not found: $CURRENT_FILE"
      fi
    done
  done

  # Commit changes if any
  if [ "$CURRENT_RELEASE_UPDATED" = true ]; then
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
      if [ -n "$EXISTING_CURRENT_PR" ] && [ "$EXISTING_CURRENT_PR" != "null|null" ]; then
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
          echo "   ⚠️  Failed to create PR for current release"
          echo "   Reason: $PR_CREATE_OUTPUT"
          CURRENT_PR_CREATED=false
        fi
      fi
    else
      echo "   ⚠️  Failed to push branch for current release PR"
      CURRENT_PR_CREATED=false
    fi
  else
    echo "   ℹ️  No updates needed for current release"
    CURRENT_PR_CREATED=false
  fi
fi

# Summary
echo ""
echo "================================================"
echo "📊 WORKFLOW SUMMARY"
echo "================================================"
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
  if [ "$RELEASE_BRANCH_EXISTS" = true ]; then
    echo "  ✓ Release branch: $RELEASE_BRANCH (already existed)"
  else
    echo "  ✓ Release branch: $RELEASE_BRANCH (created and pushed)"
  fi
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$MAIN_PR_CREATED" = true ]; then
  echo "  ✓ PR to main: ${MAIN_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$PREV_RELEASE_UPDATED" = true ] && [ "$PREV_PR_CREATED" = true ]; then
  echo "  ✓ PR to $PREV_RELEASE_BRANCH: ${PREV_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$CURRENT_RELEASE_UPDATED" = true ] && [ "$CURRENT_PR_CREATED" = true ]; then
  echo "  ✓ PR to $RELEASE_BRANCH: ${CURRENT_PR_URL}"
  COMPLETED=$((COMPLETED + 1))
fi

# Show any issues
SHOW_ISSUES=false
if [[ "$TEKTON_UPDATED" = false || "$CONTAINERFILE_UPDATED" = false || \
   "$MAIN_PR_CREATED" = false || "$RELEASE_BRANCH_PUSHED" = false ]]; then
  SHOW_ISSUES=true
fi

if [ "$SHOW_ISSUES" = true ]; then
  echo ""
  echo "⚠️  ISSUES / WARNINGS:"

  if [ "$TEKTON_UPDATED" = false ]; then
    echo "  ⚠ .tekton/ files not created"
    FAILED=$((FAILED + 1))
  fi

  if [ "$CONTAINERFILE_UPDATED" = false ]; then
    echo "  ⚠ Containerfile labels not updated"
    FAILED=$((FAILED + 1))
  fi

  if [ "$MAIN_PR_CREATED" = false ] && [ "$MAIN_CHANGES_COMMITTED" = true ]; then
    echo "  ⚠ PR to main not created (manual creation needed)"
    FAILED=$((FAILED + 1))
  fi

  if [ "$RELEASE_BRANCH_PUSHED" = false ] && [ "$RELEASE_BRANCH_EXISTS" = false ]; then
    echo "  ⚠ Release branch not pushed (manual push needed)"
    FAILED=$((FAILED + 1))
  fi

  if [ "$PREV_RELEASE_UPDATED" = false ] && [ "$PREV_RELEASE_EXISTS" = true ]; then
    echo "  ℹ️  Previous release not updated (may already be correct)"
  fi
fi

echo ""
echo "================================================"
echo "📝 NEXT STEPS"
echo "================================================"

PR_COUNT=0
if [ "$MAIN_PR_CREATED" = true ]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${MAIN_PR_URL}"
fi

if [ "$PREV_PR_CREATED" = true ]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${PREV_PR_URL}"
fi

if [ "$CURRENT_PR_CREATED" = true ]; then
  PR_COUNT=$((PR_COUNT + 1))
  echo "${PR_COUNT}. Review and merge: ${CURRENT_PR_URL}"
fi

echo ""
echo "After merge: Verify Konflux pipelines and builds"

echo ""
echo "================================================"
if [ $FAILED -eq 0 ]; then
  echo "✅ SUCCESS ($COMPLETED tasks completed)"
else
  echo "⚠️  COMPLETED WITH WARNINGS ($COMPLETED completed, $FAILED warnings)"
fi
echo "================================================"
