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
REPO_URL="https://github.com/${REPO_ORG}/${REPO_NAME}.git"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$GH_VERSION_SHORT" ] || [ -z "$ACM_VERSION" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GH_VERSION_SHORT, ACM_VERSION"
  exit 1
fi

# Allow using current directory if explicitly requested
CURRENT_DIR="$(pwd)"
USE_CURRENT_DIR="${USE_CURRENT_DIR:-no}"

if [ "$USE_CURRENT_DIR" = "yes" ]; then
  # User explicitly wants to use current directory
  WORK_DIR="$CURRENT_DIR"
  echo "✨ Using current directory as work directory: $WORK_DIR"
else
  # Default: use temporary directory
  WORK_DIR="${WORK_DIR:-/tmp/multicluster-global-hub-release}"
fi

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
echo "Workflow:"
echo "  1. Update main branch with new .tekton files (target_branch=main)"
echo "  2. Create release branch from updated main"
echo "  3. Create PR to upstream main with new release configurations"
echo "  4. Update previous release .tekton files (target_branch=previous_release_branch)"
echo "================================================"

# Step 1: Clone/Update repository (idempotent)
echo ""
echo "📍 Step 1: Preparing repository..."

if [ "$USE_CURRENT_DIR" = "yes" ]; then
  echo "   ℹ️  Using current directory (skipping clone)"
  cd "$WORK_DIR"

  # Ensure we're on main/master and up to date
  CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
  echo "   Current branch: $CURRENT_BRANCH"

  if [ "$CURRENT_BRANCH" != "main" ] && [ "$CURRENT_BRANCH" != "master" ]; then
    echo "   Switching to main branch..."
    git checkout main 2>/dev/null || git checkout master 2>/dev/null || true
  fi

  echo "   Fetching latest changes..."
  # If upstream exists, fetch from upstream; otherwise fetch from origin
  if git remote | grep -q "^upstream$"; then
    echo "   Fetching from upstream remote..."
    git fetch upstream --prune --progress 2>&1 | grep -E "Receiving|Resolving|up to date" | head -5 || true
    echo "   Updating to latest upstream/main..."
    git pull upstream main 2>/dev/null || git pull upstream master 2>/dev/null || echo "   ℹ️  Using current state"
  else
    git fetch origin --prune --progress 2>&1 | grep -E "Receiving|Resolving|up to date" | head -5 || true
    echo "   Updating to latest..."
    git pull origin main 2>/dev/null || git pull origin master 2>/dev/null || echo "   ℹ️  Using current state"
  fi

elif [ -d "$WORK_DIR" ]; then
  echo "   Work directory exists, updating..."
  cd "$WORK_DIR"

  # Check if it's a valid git repository
  if git rev-parse --git-dir > /dev/null 2>&1; then
    echo "   Fetching latest changes (this may take a moment)..."
    if git fetch origin --prune --progress 2>&1 | grep -v "^remote:" | grep -v "^From" || true; then
      # Update main branch to latest
      echo "   Updating main branch to latest..."
      git checkout main 2>/dev/null || git checkout master 2>/dev/null || true
      git pull origin main 2>/dev/null || git pull origin master 2>/dev/null || true
      echo "   ✅ Updated to latest commit"
    else
      echo "   ⚠️  Failed to fetch, re-cloning..."
      cd ..
      rm -rf "$WORK_DIR"
      echo "   Cloning repository (--depth=1 for faster clone)..."
      git clone --depth=1 --single-branch --branch main --progress "https://github.com/${REPO_ORG}/${REPO_NAME}.git" "$WORK_DIR" 2>&1 | grep -E "Receiving|Resolving" || true
      cd "$WORK_DIR"
    fi
  else
    echo "   Invalid git directory, re-cloning..."
    cd ..
    rm -rf "$WORK_DIR"
    echo "   Cloning repository (--depth=1 for faster clone)..."
    git clone --depth=1 --single-branch --branch main --progress "https://github.com/${REPO_ORG}/${REPO_NAME}.git" "$WORK_DIR" 2>&1 | grep -E "Receiving|Resolving" || true
    cd "$WORK_DIR"
  fi
else
  echo "   Cloning ${REPO_ORG}/${REPO_NAME} (--depth=1 for faster clone)..."
  git clone --depth=1 --single-branch --branch main --progress "https://github.com/${REPO_ORG}/${REPO_NAME}.git" "$WORK_DIR" 2>&1 | grep -E "Receiving|Resolving" || true
  cd "$WORK_DIR"
fi

echo "   ✅ Repository ready at $WORK_DIR"

# Step 2: Detect previous release version for migration
echo ""
echo "📍 Step 2: Detecting previous release version..."
echo "   Fetching release branches info..."

# Determine which remote to use for release branches
if git remote | grep -q "^upstream$"; then
  REMOTE="upstream"
  echo "   Fetching from upstream remote..."
  git fetch upstream 'refs/heads/release-*:refs/remotes/upstream/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
  LATEST_RELEASE=$(git branch -r | grep -E 'upstream/release-[0-9]+\.[0-9]+$' | sed 's|.*upstream/||' | sed 's|^[* ]*||' | sort -V | tail -1)
else
  REMOTE="origin"
  git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
  LATEST_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)
fi

if [ -z "$LATEST_RELEASE" ]; then
  echo "   ❌ Error: Could not detect latest release branch"
  exit 1
fi

echo "   Previous release: $LATEST_RELEASE"

# Extract previous version info for file migrations
PREV_VERSION=$(echo "$LATEST_RELEASE" | sed 's/release-//')
PREV_MINOR=$(echo "$PREV_VERSION" | cut -d. -f2)
PREV_GH_MINOR=$((PREV_MINOR - 9))
PREV_GH_VERSION_SHORT="1.${PREV_GH_MINOR}"

echo "   Current release: $RELEASE_BRANCH"
echo "   ACM Version: $(echo "$RELEASE_BRANCH" | sed 's/release-//')"
echo "   Global Hub Version: $GH_VERSION (Short: $GH_VERSION_SHORT)"
echo "   Previous Global Hub: ${PREV_GH_VERSION_SHORT} -> New: ${GH_VERSION_SHORT}"

# Step 3: Prepare working branch for main PR
echo ""
echo "📍 Step 3: Preparing branch for PR to main..."

# Ensure we're on latest main
git checkout main 2>/dev/null || git checkout master 2>/dev/null || true
git pull "$REMOTE" main 2>/dev/null || git pull "$REMOTE" master 2>/dev/null || true

# Create working branch for main PR
MAIN_PR_BRANCH="release-${ACM_VERSION}-tekton-configs"
echo "   Creating branch for main PR: $MAIN_PR_BRANCH"
git checkout -B "$MAIN_PR_BRANCH" "$REMOTE/main"

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

  echo "   Creating new files for $NEW_TAG (keeping $PREV_TAG files)..."

  # Process each component (agent, manager, operator)
  for component in agent manager operator; do
    for pipeline_type in pull-request push; do
      OLD_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"
      NEW_FILE=".tekton/multicluster-global-hub-${component}-${NEW_TAG}-${pipeline_type}.yaml"

      # Check if new file already exists
      if [ -f "$NEW_FILE" ]; then
        echo "   ℹ️  Already exists: $NEW_FILE"

        # Verify content is correct (target_branch should be "main")
        if grep -q 'target_branch == "main"' "$NEW_FILE" && \
           grep -q "${NEW_TAG}" "$NEW_FILE"; then
          echo "   ✓ Content verified for $NEW_FILE"
        else
          echo "   ⚠️  Content may need manual review: $NEW_FILE"
        fi
        TEKTON_UPDATED=true
        continue
      fi

      # Old file exists, copy to new file
      if [ -f "$OLD_FILE" ]; then
        # Copy old file to new file
        cp "$OLD_FILE" "$NEW_FILE"
        echo "   ✅ Created: $NEW_FILE (from $OLD_FILE)"

        # Update content in the new file (keep target_branch as "main")
        # 1. Update application and component labels
        sed "${SED_INPLACE[@]}" "s/release-${PREV_TAG}/release-${NEW_TAG}/g" "$NEW_FILE"
        sed "${SED_INPLACE[@]}" "s/${component}-${PREV_TAG}/${component}-${NEW_TAG}/g" "$NEW_FILE"

        # 2. Update name
        sed "${SED_INPLACE[@]}" "s/name: multicluster-global-hub-${component}-${PREV_TAG}/name: multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        # 3. Update service account name
        sed "${SED_INPLACE[@]}" "s/build-pipeline-multicluster-global-hub-${component}-${PREV_TAG}/build-pipeline-multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        # 4. Ensure target_branch remains "main" (no change needed, but verify)
        if ! grep -q 'target_branch == "main"' "$NEW_FILE"; then
          echo "   ⚠️  Warning: target_branch not set to main in $NEW_FILE"
        fi

        git add "$NEW_FILE"
        echo "   ✅ Updated content in $NEW_FILE (target_branch=main)"
        TEKTON_UPDATED=true
      else
        echo "   ⚠️  Source file not found: $OLD_FILE"
      fi
    done
  done

  if [ "$TEKTON_UPDATED" = true ]; then
    echo "   ✅ New .tekton/ configuration files created"
  else
    echo "   ⚠️  No .tekton/ files were created"
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
  echo "   ✅ Containerfile version labels processed"
else
  echo "   ⚠️  No Containerfile updates were made"
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
  git add */Containerfile.* 2>/dev/null || true

  git commit -m "Add ${RELEASE_BRANCH} pipeline configurations

- Add new .tekton/ pipelines for ${NEW_TAG} (target_branch=main)
- Update Containerfile version labels to release-${GH_VERSION_SHORT}
- Keep existing ${PREV_TAG} pipelines for backward compatibility

Release: ACM ${ACM_VERSION} / Global Hub ${GH_VERSION}"

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
  echo "   Checking out existing release branch..."
  git checkout -B "$RELEASE_BRANCH" "$UPSTREAM_REMOTE/$RELEASE_BRANCH"

  # Merge changes from main PR branch if there are updates
  if [ "$MAIN_CHANGES_COMMITTED" = true ]; then
    echo "   Merging latest changes from $MAIN_PR_BRANCH..."
    git merge --no-ff "$MAIN_PR_BRANCH" -m "Merge latest configurations for ${RELEASE_BRANCH}" || {
      echo "   ⚠️  Merge conflict or failed, manual intervention needed"
    }
  fi
else
  echo "   Creating new release branch from $MAIN_PR_BRANCH..."
  git checkout -b "$RELEASE_BRANCH" "$MAIN_PR_BRANCH"
  echo "   ✅ Created release branch: $RELEASE_BRANCH"
fi

# Get GitHub user from origin for fork detection
GITHUB_USER=$(git remote get-url origin | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')

RELEASE_BRANCH_PUSHED=false
if [ "$GITHUB_USER" = "$REPO_ORG" ]; then
  echo "   ✅ Working directly on ${REPO_ORG} repository"

  # Push release branch directly to upstream
  if [ "$MAIN_CHANGES_COMMITTED" = true ] || [ "$RELEASE_BRANCH_EXISTS" = false ]; then
    echo "   Pushing ${RELEASE_BRANCH} to $UPSTREAM_REMOTE..."
    if git push "$UPSTREAM_REMOTE" "$RELEASE_BRANCH" 2>&1; then
      echo "   ✅ Release branch pushed: ${RELEASE_BRANCH}"
      RELEASE_BRANCH_PUSHED=true
    else
      echo "   ⚠️  Failed to push release branch"
    fi
  fi
else
  echo "   ℹ️  Working with fork: ${GITHUB_USER}"
  echo "   Note: Release branch created locally, push to upstream manually if needed"
fi

# Step 8: Push PR branch and create PR to main
echo ""
echo "📍 Step 8: Creating PR to main branch..."

# Check if PR already exists
echo "   Checking for existing PR to main..."
EXISTING_MAIN_PR=$(gh pr list \
  --repo "${REPO_ORG}/${REPO_NAME}" \
  --head "${GITHUB_USER}:${MAIN_PR_BRANCH}" \
  --base main \
  --state all \
  --json number,url,state \
  --jq '.[0] | "\(.state)|\(.url)"' 2>/dev/null || echo "")

if [ -n "$EXISTING_MAIN_PR" ]; then
  MAIN_PR_STATE=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f1)
  MAIN_PR_URL=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f2)
  echo "   ℹ️  PR already exists (state: $MAIN_PR_STATE): $MAIN_PR_URL"
  MAIN_PR_CREATED=true
else
  # Push PR branch to fork/origin
  if [ "$MAIN_CHANGES_COMMITTED" = true ]; then
    git checkout "$MAIN_PR_BRANCH"
    echo "   Pushing $MAIN_PR_BRANCH to origin..."
    if git push -f origin "$MAIN_PR_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin"

      # Create PR to main
      echo "   Creating PR to ${REPO_ORG}:main..."
      MAIN_PR_URL=$(gh pr create \
        --repo "${REPO_ORG}/${REPO_NAME}" \
        --base main \
        --head "${GITHUB_USER}:${MAIN_PR_BRANCH}" \
        --title "Add ${RELEASE_BRANCH} pipeline configurations" \
        --body "## Summary

Add new pipeline configurations for ${RELEASE_BRANCH} to the main branch.

## Changes

- Add new .tekton/ pipeline files for \`${NEW_TAG}\` with \`target_branch=main\`
- Update Containerfile version labels to \`release-${GH_VERSION_SHORT}\`
- Keep existing \`${PREV_TAG}\` pipelines for backward compatibility

## Release Info

- **Release Branch**: ${RELEASE_BRANCH}
- **ACM Version**: ${ACM_VERSION}
- **Global Hub Version**: ${GH_VERSION}

## Note

This PR adds the new release pipeline configurations to main branch while preserving the existing pipelines." 2>&1) || true

      if [ -n "$MAIN_PR_URL" ] && [[ ! "$MAIN_PR_URL" =~ "error" ]]; then
        echo "   ✅ PR created: $MAIN_PR_URL"
        MAIN_PR_CREATED=true
      else
        echo "   ⚠️  Failed to create PR automatically"
        echo "   Error: $MAIN_PR_URL"
        MAIN_PR_CREATED=false
      fi
    else
      echo "   ⚠️  Failed to push branch"
      MAIN_PR_CREATED=false
    fi
  else
    echo "   ℹ️  No changes to push"
    MAIN_PR_CREATED=false
  fi
fi

# Step 9: Update previous release .tekton files to point to their release branch
echo ""
echo "📍 Step 9: Updating previous release .tekton files..."

# Calculate previous release branch
PREV_ACM_VERSION=$(echo "$LATEST_RELEASE" | sed 's/release-//')
PREV_RELEASE_BRANCH="release-${PREV_ACM_VERSION}"

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
    git commit -m "Update ${PREV_TAG} pipelines to target ${PREV_RELEASE_BRANCH}

- Update .tekton/ target_branch from main to ${PREV_RELEASE_BRANCH}
- Ensures pipelines trigger for the correct release branch

Previous Release: ACM ${PREV_ACM_VERSION} / Global Hub release-${PREV_GH_VERSION_SHORT}"

    echo "   ✅ Changes committed to $PREV_RELEASE_BRANCH"

    # Push changes if we have write access
    if [ "$GITHUB_USER" = "$REPO_ORG" ]; then
      echo "   Pushing $PREV_RELEASE_BRANCH to $UPSTREAM_REMOTE..."
      if git push "$UPSTREAM_REMOTE" "$PREV_RELEASE_BRANCH" 2>&1; then
        echo "   ✅ Previous release branch updated: ${PREV_RELEASE_BRANCH}"
      else
        echo "   ⚠️  Failed to push previous release branch"
      fi
    else
      echo "   ℹ️  Working with fork, manual push needed for $PREV_RELEASE_BRANCH"
    fi
  else
    echo "   ℹ️  No updates needed for previous release"
  fi
fi

# Summary
echo ""
echo "================================================"
echo "📊 WORKFLOW SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo "Previous: $PREV_RELEASE_BRANCH (Global Hub release-${PREV_GH_VERSION_SHORT})"
echo ""

# Count tasks
COMPLETED=0
FAILED=0

echo "✅ COMPLETED TASKS:"

if [ "$TEKTON_UPDATED" = true ] || [ "$CONTAINERFILE_UPDATED" = true ]; then
  echo "  ✓ Created new configuration files on main branch:"
  if [ "$TEKTON_UPDATED" = true ]; then
    echo "    - New .tekton/ files: ${NEW_TAG} (target_branch=main)"
  fi
  if [ "$CONTAINERFILE_UPDATED" = true ]; then
    echo "    - Containerfile labels: release-${GH_VERSION_SHORT}"
  fi
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$RELEASE_BRANCH_PUSHED" = true ] || [ "$RELEASE_BRANCH_EXISTS" = true ]; then
  if [ "$RELEASE_BRANCH_EXISTS" = true ]; then
    echo "  ✓ Release branch: $RELEASE_BRANCH (already existed)"
  else
    echo "  ✓ Release branch: $RELEASE_BRANCH (created and pushed)"
  fi
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$MAIN_PR_CREATED" = true ]; then
  echo "  ✓ PR to main: Created"
  echo "    URL: ${MAIN_PR_URL}"
  echo "    - New .tekton/ files with target_branch=main"
  echo "    - Updated Containerfile labels"
  COMPLETED=$((COMPLETED + 1))
fi

if [ "$PREV_RELEASE_UPDATED" = true ]; then
  echo "  ✓ Previous release updated: $PREV_RELEASE_BRANCH"
  echo "    - Updated ${PREV_TAG} files: target_branch=${PREV_RELEASE_BRANCH}"
  COMPLETED=$((COMPLETED + 1))
fi

# Show any issues
SHOW_ISSUES=false
if [ "$TEKTON_UPDATED" = false ] || [ "$CONTAINERFILE_UPDATED" = false ] || \
   [ "$MAIN_PR_CREATED" = false ] || [ "$RELEASE_BRANCH_PUSHED" = false ]; then
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

if [ "$MAIN_PR_CREATED" = true ]; then
  echo ""
  echo "1. Review and merge PR to main:"
  echo "   ${MAIN_PR_URL}"
fi

if [ "$GITHUB_USER" != "$REPO_ORG" ]; then
  echo ""
  echo "2. Manual actions needed (fork workflow):"
  if [ "$RELEASE_BRANCH_PUSHED" = false ]; then
    echo "   - Push release branch to upstream: git push upstream ${RELEASE_BRANCH}"
  fi
  if [ "$PREV_RELEASE_UPDATED" = true ]; then
    echo "   - Push previous release branch: git push upstream ${PREV_RELEASE_BRANCH}"
  fi
fi

echo ""
echo "3. Verify after merge:"
echo "   - Check Konflux for new pipeline runs"
echo "   - Verify tekton pipelines trigger correctly"
echo "   - Confirm new release branch builds successfully"

echo ""
echo "================================================"
if [ $FAILED -eq 0 ]; then
  echo "✅ SUCCESS ($COMPLETED tasks completed)"
else
  echo "⚠️  COMPLETED WITH WARNINGS ($COMPLETED completed, $FAILED warnings)"
fi
echo "================================================"
