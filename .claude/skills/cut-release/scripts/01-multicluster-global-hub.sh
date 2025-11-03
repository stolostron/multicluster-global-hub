#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Repository Release Script
# Creates release branch in stolostron/multicluster-global-hub,
# updates .tekton/ and Containerfile configurations, and creates PR
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   GH_VERSION_SHORT  - Short Global Hub version (e.g., 1.8)

# Configuration
REPO_ORG="${REPO_ORG:-stolostron}"
REPO_NAME="${REPO_NAME:-multicluster-global-hub}"
REPO_URL="https://github.com/${REPO_ORG}/${REPO_NAME}.git"

# Validate required environment variables
if [ -z "$RELEASE_BRANCH" ] || [ -z "$GH_VERSION" ] || [ -z "$GH_VERSION_SHORT" ]; then
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GH_VERSION_SHORT"
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

echo "🚀 Multicluster Global Hub Release Branch Creation"
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

# Step 3: Create/checkout release branch (idempotent)
echo ""
echo "📍 Step 3: Creating/checking out release branch..."

# Check if branch exists on remote (use upstream if available, otherwise origin)
REMOTE_BRANCH_EXISTS=false
if git ls-remote --heads "$REMOTE" "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  REMOTE_BRANCH_EXISTS=true
  echo "   ℹ️  Branch $RELEASE_BRANCH already exists on $REMOTE remote"
fi

# Check if branch exists locally
LOCAL_BRANCH_EXISTS=false
if git show-ref --verify --quiet "refs/heads/$RELEASE_BRANCH"; then
  LOCAL_BRANCH_EXISTS=true
  echo "   ℹ️  Branch $RELEASE_BRANCH exists locally"
fi

# Checkout or create the branch
if [ "$LOCAL_BRANCH_EXISTS" = true ]; then
  echo "   Checking out existing local branch..."
  git checkout "$RELEASE_BRANCH"

  if [ "$REMOTE_BRANCH_EXISTS" = true ]; then
    echo "   Pulling latest changes from $REMOTE/$RELEASE_BRANCH..."
    git pull "$REMOTE" "$RELEASE_BRANCH" || echo "   ⚠️  Failed to pull, continuing with local branch"
  fi
elif [ "$REMOTE_BRANCH_EXISTS" = true ]; then
  echo "   Checking out remote branch from $REMOTE..."
  git checkout -b "$RELEASE_BRANCH" "$REMOTE/$RELEASE_BRANCH"
else
  echo "   Creating new branch from $REMOTE/main..."
  git checkout -b "$RELEASE_BRANCH" "$REMOTE/main"
  echo "   ✅ Created new branch $RELEASE_BRANCH"
fi

# Step 4: Update .tekton/ configurations (idempotent)
echo ""
echo "📍 Step 4: Updating .tekton/ pipelinesascode configurations..."

TEKTON_UPDATED=false

if [ ! -d ".tekton" ]; then
  echo "   ⚠️  .tekton/ directory not found in current repository"
else
  # Find all .yaml files in .tekton/ directory with previous version
  PREV_TAG="globalhub-${PREV_GH_VERSION_SHORT//./-}"
  NEW_TAG="globalhub-${GH_VERSION_SHORT//./-}"

  echo "   Processing files from $PREV_TAG to $NEW_TAG..."

  # Process each component (agent, manager, operator)
  for component in agent manager operator; do
    for pipeline_type in pull-request push; do
      OLD_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"
      NEW_FILE=".tekton/multicluster-global-hub-${component}-${NEW_TAG}-${pipeline_type}.yaml"

      # Check if already updated (new file exists and old file doesn't)
      if [ -f "$NEW_FILE" ] && [ ! -f "$OLD_FILE" ]; then
        echo "   ℹ️  Already updated: $NEW_FILE"

        # Verify content is correct (idempotent check)
        if grep -q "target_branch == \"${RELEASE_BRANCH}\"" "$NEW_FILE" && \
           grep -q "${NEW_TAG}" "$NEW_FILE"; then
          echo "   ✓ Content verified for $NEW_FILE"
        else
          echo "   ⚠️  Content may need manual review: $NEW_FILE"
        fi
        TEKTON_UPDATED=true
        continue
      fi

      # Old file exists, need to rename and update
      if [ -f "$OLD_FILE" ]; then
        # Rename the file
        git mv "$OLD_FILE" "$NEW_FILE" 2>/dev/null || {
          echo "   ⚠️  Failed to git mv, trying regular mv..."
          mv "$OLD_FILE" "$NEW_FILE"
          git add "$NEW_FILE"
        }
        echo "   ✅ Renamed: $OLD_FILE -> $NEW_FILE"

        # Update content in the renamed file
        # 1. Update target_branch from "main" to release branch
        sed "${SED_INPLACE[@]}" "s/target_branch == \"main\"/target_branch == \"${RELEASE_BRANCH}\"/" "$NEW_FILE"

        # 2. Update application and component labels
        sed "${SED_INPLACE[@]}" "s/release-${PREV_TAG}/release-${NEW_TAG}/g" "$NEW_FILE"
        sed "${SED_INPLACE[@]}" "s/${component}-${PREV_TAG}/${component}-${NEW_TAG}/g" "$NEW_FILE"

        # 3. Update name
        sed "${SED_INPLACE[@]}" "s/name: multicluster-global-hub-${component}-${PREV_TAG}/name: multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        # 4. Update service account name
        sed "${SED_INPLACE[@]}" "s/build-pipeline-multicluster-global-hub-${component}-${PREV_TAG}/build-pipeline-multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

        echo "   ✅ Updated content in $NEW_FILE"
        TEKTON_UPDATED=true
      elif [ ! -f "$NEW_FILE" ]; then
        echo "   ⚠️  Neither old nor new file found for ${component}-${pipeline_type}"
      fi
    done
  done

  if [ "$TEKTON_UPDATED" = true ]; then
    echo "   ✅ .tekton/ configurations processed"
  else
    echo "   ⚠️  No .tekton/ files needed updates"
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

# Step 6: Commit changes
echo ""
echo "📍 Step 6: Committing changes..."

# Check for staged and unstaged changes
if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  CHANGES_COMMITTED=false
else
  # Stage all changes (git mv already stages renamed files)
  git add .tekton/ 2>/dev/null || true
  git add */Containerfile.* 2>/dev/null || true

  git commit -m "Update configurations for ${RELEASE_BRANCH}

- Rename and update .tekton/ pipelines from ${PREV_TAG} to ${NEW_TAG}
- Update .tekton/ target_branch from main to ${RELEASE_BRANCH}
- Update Containerfile version labels to release-${GH_VERSION_SHORT}

Release: ACM $(echo "$RELEASE_BRANCH" | sed 's/release-//') / Global Hub ${GH_VERSION}"

  echo "   ✅ Changes committed"
  CHANGES_COMMITTED=true
fi

# Step 7: Push release branch (idempotent)
echo ""
echo "📍 Step 7: Pushing release branch to ${REPO_ORG}/${REPO_NAME}..."

# Get GitHub user from origin
GITHUB_USER=$(git remote get-url origin | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|')

if [ "$GITHUB_USER" = "$REPO_ORG" ]; then
  echo "   ✅ Working directly on ${REPO_ORG} repository"
  echo "   Will push release branch directly to origin"

  # Push release branch directly to origin
  if [ "$CHANGES_COMMITTED" = true ] || [ "$TEKTON_UPDATED" = true ] || [ "$CONTAINERFILE_UPDATED" = true ]; then
    git checkout "$RELEASE_BRANCH"
    echo "   Pushing ${RELEASE_BRANCH} to origin..."
    if git push origin "$RELEASE_BRANCH" 2>&1; then
      echo "   ✅ Release branch pushed: ${RELEASE_BRANCH}"
      RELEASE_BRANCH_PUSHED=true
    else
      echo "   ⚠️  Failed to push release branch"
      RELEASE_BRANCH_PUSHED=false
    fi
  else
    echo "   ℹ️  No changes to push"
    RELEASE_BRANCH_PUSHED=false
  fi

  # No PR needed for release branch when working directly on stolostron
  PR1_CREATED=false
  PR1_URL=""
else
  # Working with fork - need to create PR to upstream release branch
  echo "   ⚠️  Working with fork: ${GITHUB_USER}"
  echo "   Note: This workflow expects direct push to ${REPO_ORG}/${REPO_NAME}"
  echo "   Please ensure you have write access or use a fork workflow manually"
  PR1_CREATED=false
  PR1_URL=""
fi

# Step 8: Create PR #2 to update main branch (idempotent)
echo ""
echo "📍 Step 8: Creating PR to main branch..."

# Checkout main and create/update branch
git fetch origin main >/dev/null 2>&1
UPDATE_MAIN_BRANCH="update-main-for-${RELEASE_BRANCH}"

# Check if PR #2 already exists
echo "   Checking for existing PR to main..."
EXISTING_PR2=$(gh pr list \
  --repo "${REPO_ORG}/${REPO_NAME}" \
  --head "${GITHUB_USER}:${UPDATE_MAIN_BRANCH}" \
  --base main \
  --state all \
  --json number,url,state \
  --jq '.[0] | "\(.state)|\(.url)"' 2>/dev/null || echo "")

if [ -n "$EXISTING_PR2" ]; then
  PR2_STATE=$(echo "$EXISTING_PR2" | cut -d'|' -f1)
  PR2_URL=$(echo "$EXISTING_PR2" | cut -d'|' -f2)
  echo "   ℹ️  PR #2 already exists (state: $PR2_STATE): $PR2_URL"
  PR2_CREATED=true
  # Skip the rest of PR #2 creation
else
  # Create/checkout the update branch
  git checkout -B "$UPDATE_MAIN_BRANCH" origin/main

MAIN_UPDATED=false

# Add new .tekton pipeline files for the new release (keep old ones)
echo "   Adding new .tekton pipeline files for ${NEW_TAG}..."
for component in agent manager operator; do
  for pipeline_type in pull-request push; do
    OLD_FILE=".tekton/multicluster-global-hub-${component}-${PREV_TAG}-${pipeline_type}.yaml"
    NEW_FILE=".tekton/multicluster-global-hub-${component}-${NEW_TAG}-${pipeline_type}.yaml"

    if [ -f "$OLD_FILE" ]; then
      # Copy old file to new file
      cp "$OLD_FILE" "$NEW_FILE"

      # Update content for main branch (keep target_branch as "main")
      # Update application and component labels
      sed "${SED_INPLACE[@]}" "s/release-${PREV_TAG}/release-${NEW_TAG}/g" "$NEW_FILE"
      sed "${SED_INPLACE[@]}" "s/${component}-${PREV_TAG}/${component}-${NEW_TAG}/g" "$NEW_FILE"

      # Update name
      sed "${SED_INPLACE[@]}" "s/name: multicluster-global-hub-${component}-${PREV_TAG}/name: multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

      # Update service account name
      sed "${SED_INPLACE[@]}" "s/build-pipeline-multicluster-global-hub-${component}-${PREV_TAG}/build-pipeline-multicluster-global-hub-${component}-${NEW_TAG}/" "$NEW_FILE"

      git add "$NEW_FILE"
      echo "   ✅ Added: $NEW_FILE"
      MAIN_UPDATED=true
    fi
  done
done

# Update Containerfile version labels
echo "   Updating Containerfile version labels..."
for component in agent manager operator; do
  CONTAINERFILE="${component}/Containerfile.${component}"

  if [ -f "$CONTAINERFILE" ]; then
    if grep -q "LABEL version=\"release-${PREV_GH_VERSION_SHORT}\"" "$CONTAINERFILE"; then
      sed "${SED_INPLACE[@]}" "s/LABEL version=\"release-${PREV_GH_VERSION_SHORT}\"/LABEL version=\"release-${GH_VERSION_SHORT}\"/" "$CONTAINERFILE"
      git add "$CONTAINERFILE"
      echo "   ✅ Updated $CONTAINERFILE"
      MAIN_UPDATED=true
    fi
  fi
done

# Commit and create PR to main
if [ "$MAIN_UPDATED" = true ]; then
  git commit -m "Add ${RELEASE_BRANCH} pipeline configurations

- Add new .tekton/ pipelines for ${NEW_TAG} (main branch)
- Update Containerfile version labels to release-${GH_VERSION_SHORT}
- Keep existing ${PREV_TAG} pipelines for backward compatibility

Release: ACM $(echo "$RELEASE_BRANCH" | sed 's/release-//') / Global Hub ${GH_VERSION}"

  echo "   Pushing to fork..."
  git push -f origin "update-main-for-${RELEASE_BRANCH}"

  # Create PR to main
  echo "   Creating PR to ${REPO_ORG}:main..."
  PR2_URL=$(gh pr create \
    --repo "${REPO_ORG}/${REPO_NAME}" \
    --base main \
    --head "${GITHUB_USER}:update-main-for-${RELEASE_BRANCH}" \
    --title "Add ${RELEASE_BRANCH} pipeline configurations to main" \
    --body "## Summary

Add new pipeline configurations for ${RELEASE_BRANCH} to the main branch.

## Changes

- Add new .tekton/ pipeline files for \`${NEW_TAG}\`
- Update Containerfile version labels to \`release-${GH_VERSION_SHORT}\`
- Keep existing \`${PREV_TAG}\` pipelines for backward compatibility

## Release Info

- **Release Branch**: ${RELEASE_BRANCH}
- **ACM Version**: $(echo "$RELEASE_BRANCH" | sed 's/release-//')
- **Global Hub Version**: ${GH_VERSION}

## Note

This PR adds the new release pipeline configurations to main branch while preserving the existing pipelines. The old pipelines can be removed in a future cleanup." 2>&1) || true

    if [ -n "$PR2_URL" ] && [[ ! "$PR2_URL" =~ "error" ]]; then
      echo "   ✅ PR #2 created: $PR2_URL"
      PR2_CREATED=true
    else
      echo "   ⚠️  Failed to create PR #2 automatically"
      echo "   Error: $PR2_URL"
      PR2_CREATED=false
    fi
  else
    echo "   ℹ️  No changes needed for main branch"
    PR2_CREATED=false
  fi
fi  # Close the if for EXISTING_PR2 check

# Summary
echo ""
echo "================================================"
echo "📊 SCRIPT SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH (Global Hub $GH_VERSION)"
echo ""

# Count tasks
COMPLETED=0
FAILED=0

echo "✅ COMPLETED TASKS:"

if [ "$REMOTE_BRANCH_EXISTS" = true ]; then
  echo "  ✓ Release branch: $RELEASE_BRANCH (already exists)"
else
  echo "  ✓ Release branch: $RELEASE_BRANCH (created from main)"
fi
COMPLETED=$((COMPLETED + 1))

# Report on release branch updates (no PR needed for direct push)
if [ "$GITHUB_USER" = "$REPO_ORG" ]; then
  if [ "$TEKTON_UPDATED" = true ] || [ "$CONTAINERFILE_UPDATED" = true ]; then
    echo "  ✓ Release branch updates pushed to ${RELEASE_BRANCH}"
    if [ "$TEKTON_UPDATED" = true ]; then
      echo "    - .tekton/ files: ${PREV_GH_VERSION_SHORT} → ${GH_VERSION_SHORT}"
    fi
    if [ "$CONTAINERFILE_UPDATED" = true ]; then
      echo "    - Containerfile labels: ${PREV_GH_VERSION_SHORT} → ${GH_VERSION_SHORT}"
    fi
    COMPLETED=$((COMPLETED + 1))
  fi
fi

if [ "$PR2_CREATED" = true ]; then
  echo "  ✓ PR to main: Created"
  echo "    - Added .tekton/ files for version ${GH_VERSION_SHORT}"
  echo "    - Containerfile labels: ${PREV_GH_VERSION_SHORT} → ${GH_VERSION_SHORT}"
  COMPLETED=$((COMPLETED + 1))
fi

# Show failures if any
SHOW_FAILURES=false
if [ "$TEKTON_UPDATED" = false ] || [ "$CONTAINERFILE_UPDATED" = false ]; then
  SHOW_FAILURES=true
fi
if [ "$PR2_CREATED" = false ] && [ "$MAIN_UPDATED" = true ]; then
  SHOW_FAILURES=true
fi

if [ "$SHOW_FAILURES" = true ]; then
  echo ""
  echo "❌ FAILED TASKS:"

  if [ "$TEKTON_UPDATED" = false ]; then
    echo "  ✗ .tekton/ files not updated"
    FAILED=$((FAILED + 1))
  fi

  if [ "$CONTAINERFILE_UPDATED" = false ]; then
    echo "  ✗ Containerfile labels not updated"
    FAILED=$((FAILED + 1))
  fi

  if [ "$PR2_CREATED" = false ] && [ "$MAIN_UPDATED" = true ]; then
    echo "  ✗ PR to main not created"
    FAILED=$((FAILED + 1))
  fi
fi

echo ""
echo "================================================"
echo "📝 MANUAL ACTIONS REQUIRED"
echo "================================================"

# PRs to review
if [ "$PR2_CREATED" = true ]; then
  echo ""
  echo "Review and merge PR:"
  echo "  - PR to main:"
  echo "    ${PR2_URL}"
fi

# Manual fixes needed
MANUAL_FIXES_NEEDED=false
if [ "$TEKTON_UPDATED" = false ] || [ "$CONTAINERFILE_UPDATED" = false ]; then
  MANUAL_FIXES_NEEDED=true
fi
if [ "$PR2_CREATED" = false ] && [ "$MAIN_UPDATED" = true ]; then
  MANUAL_FIXES_NEEDED=true
fi

if [ "$MANUAL_FIXES_NEEDED" = true ]; then
  echo ""
  echo "Manual fixes needed:"

  if [ "$TEKTON_UPDATED" = false ]; then
    echo "  - Update .tekton/ files manually in ${RELEASE_BRANCH}"
  fi

  if [ "$CONTAINERFILE_UPDATED" = false ]; then
    echo "  - Update Containerfile labels manually in ${RELEASE_BRANCH}"
  fi

  if [ "$PR2_CREATED" = false ] && [ "$MAIN_UPDATED" = true ]; then
    echo "  - Create PR to main manually"
  fi
fi

# Post-merge verification
echo ""
echo "After PRs merged:"
echo "  - Verify tekton pipelines trigger correctly"
echo "  - Check Konflux for new pipeline runs"

echo ""
echo "================================================"
if [ $FAILED -eq 0 ]; then
  echo "✅ SUCCESS ($COMPLETED tasks completed)"
else
  echo "⚠️  COMPLETED WITH ISSUES ($COMPLETED completed, $FAILED failed)"
fi
echo "================================================"
