#!/bin/bash

set -euo pipefail

# Multicluster Global Hub Operator Catalog Release Script
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
#   CATALOG_BRANCH    - Catalog branch name (e.g., release-1.8)
#   CATALOG_TAG       - Catalog tag (e.g., globalhub-1-8)
#   OCP_MIN           - Minimum OCP version number (e.g., 416)
#   OCP_MAX           - Maximum OCP version number (e.g., 420)
#   GITHUB_USER       - GitHub username for PR creation
#   CREATE_BRANCHES          - true: create branch, false: update via PR

# Configuration
CATALOG_REPO="stolostron/multicluster-global-hub-operator-catalog"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$CATALOG_BRANCH" || -z "$CATALOG_TAG" || -z "$OCP_MIN" || -z "$OCP_MAX" || -z "$GITHUB_USER" || -z "$CREATE_BRANCHES" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, CATALOG_BRANCH, CATALOG_TAG, OCP_MIN, OCP_MAX, GITHUB_USER, CREATE_BRANCHES"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)

# Constants for repeated patterns
readonly NULL_PR_VALUE='null|null'
readonly SEPARATOR_LINE='================================================'
fi

echo "🚀 Operator Catalog Release"
echo "$SEPARATOR_LINE"
echo "   Mode: $([[ "$CREATE_BRANCHES" = true ]] && echo "CUT (create branch)" || echo "UPDATE (PR only)")"
echo "   Release: $RELEASE_BRANCH / $CATALOG_BRANCH"
echo "   OCP: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"
echo ""

# Extract version for display (unused but kept for potential future use)
# shellcheck disable=SC2034
CATALOG_VERSION="${CATALOG_BRANCH#release-}"

# Setup repository
REPO_PATH="$WORK_DIR/multicluster-global-hub-operator-catalog"
mkdir -p "$WORK_DIR"

# Remove existing directory for clean clone
if [[ -d "$REPO_PATH" ]]; then
  echo "   Removing existing directory for clean clone..."
  rm -rf "$REPO_PATH"
fi

echo "📥 Cloning $CATALOG_REPO (--depth=1 for faster clone)..."
git clone --depth=1 --single-branch --branch main --progress "https://github.com/$CATALOG_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
if [[ ! -d "$REPO_PATH/.git" ]]; then
  echo "❌ Failed to clone $CATALOG_REPO" >&2
  exit 1
fi
echo "✅ Cloned successfully"

cd "$REPO_PATH"

# Setup user's fork remote
FORK_REPO="git@github.com:${GITHUB_USER}/multicluster-global-hub-operator-catalog.git"
git remote add fork "$FORK_REPO" 2>/dev/null || true

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/multicluster-global-hub-operator-catalog"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/multicluster-global-hub-operator-catalog" >&2
  echo "   Note: Cleanup PR will require manual creation if fork doesn't exist"
fi

# Fetch all release branches
echo "🔄 Fetching release branches..."
git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
echo "   ✅ Release branches fetched"

# Find latest catalog release branch
LATEST_CATALOG_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

# Check if target branch is the same as latest
if [[ "$LATEST_CATALOG_RELEASE" = "$CATALOG_BRANCH" ]]; then
  echo "ℹ️  Target catalog branch is the latest: $CATALOG_BRANCH"
  echo ""
  echo "   https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
  echo ""
  echo "   Will verify and update if needed..."
  echo ""
fi

if [[ -z "$LATEST_CATALOG_RELEASE" ]]; then
  echo "⚠️  No previous catalog release branch found, using main as base" >&2
  BASE_BRANCH="main"
else
  echo "Latest catalog release detected: $LATEST_CATALOG_RELEASE"

  # If target branch is the latest, use second-to-latest as base
  # If target branch is not the latest, use latest as base
  if [[ "$LATEST_CATALOG_RELEASE" = "$CATALOG_BRANCH" ]]; then
    # Target is latest - get second-to-latest for base
    SECOND_TO_LATEST=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
      sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
    if [[ -n "$SECOND_TO_LATEST" && "$SECOND_TO_LATEST" != "$CATALOG_BRANCH" ]]; then
      BASE_BRANCH="$SECOND_TO_LATEST"
      echo "Target is latest release, using previous release as base: $BASE_BRANCH"
    else
      BASE_BRANCH="main"
      echo "No previous release found, using main as base"
    fi
  else
    # Target is not latest - use latest as base
    BASE_BRANCH="$LATEST_CATALOG_RELEASE"
    echo "Target is older than latest, using latest as base: $BASE_BRANCH"
  fi
fi

# Extract previous catalog tag for replacements
if [[ "$BASE_BRANCH" != "main" ]]; then
  PREV_CATALOG_VERSION="${BASE_BRANCH#release-}"
  PREV_CATALOG_TAG="globalhub-${PREV_CATALOG_VERSION//./-}"
  PREV_MINOR="${PREV_CATALOG_VERSION#1.}"
  # Calculate previous OCP range using same formula as main script: OCP_BASE=10, range of 5 versions
  OCP_BASE=10
  PREV_OCP_MIN=$((4*100 + OCP_BASE + PREV_MINOR))
  PREV_OCP_MAX=$((PREV_OCP_MIN + 4))
  echo "Previous catalog: $PREV_CATALOG_VERSION (OCP 4.$((PREV_OCP_MIN%100)) - 4.$((PREV_OCP_MAX%100)))"
else
  PREV_CATALOG_TAG=""
fi

# For cleanup PR, we need to find the previous release
# If CATALOG_BRANCH is the latest, find second-to-latest for cleanup
if [[ "$LATEST_CATALOG_RELEASE" = "$CATALOG_BRANCH" ]]; then
  CLEANUP_TARGET_BRANCH=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
    sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
  if [[ -n "$CLEANUP_TARGET_BRANCH" && "$CLEANUP_TARGET_BRANCH" != "$CATALOG_BRANCH" ]]; then
    echo "Cleanup target: $CLEANUP_TARGET_BRANCH (previous release)"
  else
    CLEANUP_TARGET_BRANCH=""
  fi
else
  # When creating new catalog, BASE_BRANCH is the cleanup target
  CLEANUP_TARGET_BRANCH="$BASE_BRANCH"
fi

echo ""

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
PR_CREATED=false
PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$CATALOG_BRANCH" | grep -q "$CATALOG_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $CATALOG_BRANCH already exists on origin"
  git fetch origin "$CATALOG_BRANCH" 2>/dev/null || true
  git checkout -B "$CATALOG_BRANCH" "origin/$CATALOG_BRANCH"
else
  if [[ "$CREATE_BRANCHES" != "true" ]]; then
    echo "❌ Error: Branch $CATALOG_BRANCH does not exist on origin" >&2
    echo "   Run with CREATE_BRANCHES=true to create the branch"
    exit 1
  fi
  echo "🌿 Creating $CATALOG_BRANCH from origin/$BASE_BRANCH..."
  git branch -D "$CATALOG_BRANCH" 2>/dev/null || true
  git checkout -b "$CATALOG_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $CATALOG_BRANCH"
fi

# Step 1: Update images-mirror-set.yaml
echo ""
echo "📍 Step 1: Updating images-mirror-set.yaml..."

IMAGES_MIRROR_FILE=".tekton/images-mirror-set.yaml"
if [[ -f "$IMAGES_MIRROR_FILE" && -n "$PREV_CATALOG_TAG" ]]; then
  echo "   Updating image references in $IMAGES_MIRROR_FILE"
  echo "   Changing: *-$PREV_CATALOG_TAG"
  echo "   To:       *-$CATALOG_TAG"

  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$IMAGES_MIRROR_FILE"

  # Update OCP version references
  for ((ocp_ver=PREV_OCP_MIN%100; ocp_ver<=PREV_OCP_MAX%100; ocp_ver++)); do
    new_ocp=$((ocp_ver + 1))
    sed "${SED_INPLACE[@]}" "s/v4${ocp_ver}/v4${new_ocp}/g" "$IMAGES_MIRROR_FILE"
  done

  echo "   ✅ Updated $IMAGES_MIRROR_FILE"
elif [[ ! -f "$IMAGES_MIRROR_FILE" ]]; then
  echo "   ⚠️  File not found: $IMAGES_MIRROR_FILE" >&2
fi

# Step 2: Update OCP pipeline files
echo ""
echo "📍 Step 2: Updating OCP pipeline files..."

if [[ -n "$PREV_CATALOG_TAG" ]]; then
  NEW_OCP_VER=$((OCP_MAX%100))
  OLD_OCP_VER=$((PREV_OCP_MIN%100))

  # Check if OCP range has changed
  if [[ "$PREV_OCP_MAX" != "$OCP_MAX" ]]; then
    echo "   Adding OCP 4.${NEW_OCP_VER} pipelines..."
    echo "   Removing OCP 4.${OLD_OCP_VER} pipelines..."

    # Copy and update pull-request pipeline for new OCP version
    LATEST_PR_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}-pull-request.yaml"
    NEW_PR_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4${NEW_OCP_VER}-${CATALOG_TAG}-pull-request.yaml"

    if [[ -f "$LATEST_PR_PIPELINE" ]]; then
      cp "$LATEST_PR_PIPELINE" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/release-catalog-$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}/release-catalog-${NEW_OCP_VER}-${CATALOG_TAG}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Created $NEW_PR_PIPELINE"
    fi

    # Copy and update push pipeline for new OCP version
    LATEST_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}-push.yaml"
    NEW_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4${NEW_OCP_VER}-${CATALOG_TAG}-push.yaml"

    if [[ -f "$LATEST_PUSH_PIPELINE" ]]; then
      cp "$LATEST_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/release-catalog-$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}/release-catalog-${NEW_OCP_VER}-${CATALOG_TAG}/g" "$NEW_PUSH_PIPELINE"
      echo "   ✅ Created $NEW_PUSH_PIPELINE"
    fi
  else
    echo "   ℹ️  OCP range unchanged (4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))), skipping add/remove"
  fi

  # Update existing OCP version pipelines
  echo "   Updating existing OCP version pipelines..."
  for ((ocp_ver=(OCP_MIN%100); ocp_ver<NEW_OCP_VER; ocp_ver++)); do
    OLD_PR=".tekton/multicluster-global-hub-operator-catalog-v4${ocp_ver}-${PREV_CATALOG_TAG}-pull-request.yaml"
    NEW_PR=".tekton/multicluster-global-hub-operator-catalog-v4${ocp_ver}-${CATALOG_TAG}-pull-request.yaml"

    OLD_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${ocp_ver}-${PREV_CATALOG_TAG}-push.yaml"
    NEW_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${ocp_ver}-${CATALOG_TAG}-push.yaml"

    if [[ "$PREV_CATALOG_TAG" = "$CATALOG_TAG" ]]; then
      # Same tag - just update in place
      if [[ -f "$NEW_PR" ]]; then
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR"
        echo "   ✅ Updated v4${ocp_ver} pull-request pipeline"
      fi
      if [[ -f "$NEW_PUSH" ]]; then
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH"
        echo "   ✅ Updated v4${ocp_ver} push pipeline"
      fi
    else
      # Different tags - rename and update
      if [[ -f "$OLD_PR" ]]; then
        git mv "$OLD_PR" "$NEW_PR" 2>/dev/null || cp "$OLD_PR" "$NEW_PR"
        sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR"
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR"
        echo "   ✅ Updated v4${ocp_ver} pull-request pipeline"
      fi

      if [[ -f "$OLD_PUSH" ]]; then
        git mv "$OLD_PUSH" "$NEW_PUSH" 2>/dev/null || cp "$OLD_PUSH" "$NEW_PUSH"
        sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH"
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH"
        echo "   ✅ Updated v4${ocp_ver} push pipeline"
      fi
    fi
  done

  # Remove old OCP version pipelines (only if OCP range changed)
  if [[ "$PREV_OCP_MAX" != "$OCP_MAX" ]]; then
    OLD_OCP_PR=".tekton/multicluster-global-hub-operator-catalog-v4${OLD_OCP_VER}-${PREV_CATALOG_TAG}-pull-request.yaml"
    OLD_OCP_PUSH=".tekton/multicluster-global-hub-operator-catalog-v4${OLD_OCP_VER}-${PREV_CATALOG_TAG}-push.yaml"

    if [[ -f "$OLD_OCP_PR" ]]; then
      git rm "$OLD_OCP_PR" 2>/dev/null || rm "$OLD_OCP_PR"
      echo "   ✅ Removed old OCP 4.${OLD_OCP_VER} pull-request pipeline"
    fi

    if [[ -f "$OLD_OCP_PUSH" ]]; then
      git rm "$OLD_OCP_PUSH" 2>/dev/null || rm "$OLD_OCP_PUSH"
      echo "   ✅ Removed old OCP 4.${OLD_OCP_VER} push pipeline"
    fi
  fi
fi

# Step 3: Commit changes
echo ""
echo "📍 Step 3: Committing changes on $CATALOG_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  CHANGES_COMMITTED=false
else
  git add -A

  COMMIT_MSG="Update catalog for ${CATALOG_BRANCH} (Global Hub ${GH_VERSION})

- Update images-mirror-set.yaml to use ${CATALOG_TAG}
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Remove OCP 4.${OLD_OCP_VER} pipelines
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines

Supports OCP 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit --signoff -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
  CHANGES_COMMITTED=true
fi

  # Step 4: Push to origin or create PR
  echo ""
  echo "📍 Step 4: Publishing changes..."

  if [[ "$CHANGES_COMMITTED" = false ]]; then
    echo "   ℹ️  No changes to publish"
  else
    # Decision: Push directly or create PR based on CREATE_BRANCHES and branch existence
    if [[ "$CREATE_BRANCHES" = "true" && "$BRANCH_EXISTS_ON_ORIGIN" = false ]]; then
      # CUT mode + branch doesn't exist - push directly
      echo "   Pushing new branch $CATALOG_BRANCH to origin..."
      if git push origin "$CATALOG_BRANCH" 2>&1; then
        echo "   ✅ Branch pushed to origin: $CATALOG_REPO/$CATALOG_BRANCH"
        PUSHED_TO_ORIGIN=true
      else
        echo "   ❌ Failed to push branch to origin" >&2
        exit 1
      fi
    else
      # Branch exists or UPDATE mode - create PR to update it
      echo "   Creating PR to update $CATALOG_BRANCH..."

      # Check if PR already exists
      echo "   Checking for existing PR to $CATALOG_BRANCH..."
      EXISTING_PR=$(gh pr list \
        --repo "${CATALOG_REPO}" \
        --base "$CATALOG_BRANCH" \
        --state open \
        --search "Update ${CATALOG_BRANCH} catalog configuration" \
        --json number,url,state \
        --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

      if [[ -n "$EXISTING_PR" && "$EXISTING_PR" != "$NULL_PR_VALUE" ]]; then
        PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
        PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
        echo "   ℹ️  PR already exists (state: $PR_STATE): $PR_URL"
        PR_CREATED=true
      else
        PR_BRANCH="${CATALOG_BRANCH}-update-$(date +%s)"
        git checkout -b "$PR_BRANCH"

        if [[ "$FORK_EXISTS" = false ]]; then
          echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
          echo "   Please fork ${CATALOG_REPO} to enable PR creation"
          PR_CREATED=false
        else
          echo "   Pushing $PR_BRANCH to fork..."
          if git push -f fork "$PR_BRANCH" 2>&1; then
            echo "   ✅ PR branch pushed to fork"

          PR_BODY="Update ${CATALOG_BRANCH} catalog configuration

## Changes

- Update images-mirror-set.yaml to use \`${CATALOG_TAG}\`
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Remove OCP 4.${OLD_OCP_VER} pipelines
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines

## Version Mapping

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: ${GH_VERSION}
- **Catalog branch**: ${CATALOG_BRANCH}
- **Supported OCP**: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
- **Previous release**: ${BASE_BRANCH}"

          PR_CREATE_OUTPUT=$(gh pr create --base "$CATALOG_BRANCH" --head "${GITHUB_USER}:$PR_BRANCH" \
            --title "Update ${CATALOG_BRANCH} catalog configuration" \
            --body "$PR_BODY" \
            --repo "$CATALOG_REPO" 2>&1) || true

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

# Step 5: Create cleanup PR to remove GitHub Actions from old release
echo ""
echo "📍 Step 5: Creating cleanup PR for old release..."

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

Workflow has been moved to ${CATALOG_BRANCH}.
This prevents duplicate automation on old release branch."

    git commit --signoff -m "$CLEANUP_COMMIT_MSG"
    echo "   ✅ Committed workflow removal"

    # Check if cleanup PR already exists
    echo "   Checking for existing cleanup PR..."
    EXISTING_CLEANUP_PR=$(gh pr list \
      --repo "${CATALOG_REPO}" \
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
        echo "   Please fork ${CATALOG_REPO} and run again, or create cleanup PR manually"
        CLEANUP_PR_CREATED=false
      elif git push -f fork "$CLEANUP_BRANCH" 2>&1; then
        echo "   ✅ Cleanup branch pushed to fork"

        # Create cleanup PR to old release branch
        CLEANUP_PR_BODY="Remove GitHub Actions workflow from ${CLEANUP_TARGET_BRANCH}

The workflow has been moved to the new release branch \`${CATALOG_BRANCH}\`.

This PR removes the workflow from ${CLEANUP_TARGET_BRANCH} to prevent duplicate automation."

        CLEANUP_PR_OUTPUT=$(gh pr create --base "$CLEANUP_TARGET_BRANCH" --head "${GITHUB_USER}:$CLEANUP_BRANCH" \
          --title "Remove GitHub Actions from ${CLEANUP_TARGET_BRANCH}" \
          --body "$CLEANUP_PR_BODY" \
          --repo "$CATALOG_REPO" 2>&1) || true

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
echo "Release: $RELEASE_BRANCH / $CATALOG_BRANCH"
echo "Supported OCP: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Catalog branch: $CATALOG_BRANCH (from $BASE_BRANCH)"
if [[ "$CHANGES_COMMITTED" = true ]]; then
  echo "  ✓ Updated images-mirror-set.yaml to ${CATALOG_TAG}"
  if [[ -n "$PREV_CATALOG_TAG" ]]; then
    echo "  ✓ Added OCP 4.${NEW_OCP_VER} pipelines"
    echo "  ✓ Removed OCP 4.${OLD_OCP_VER} pipelines"
    echo "  ✓ Updated OCP 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100-1)) pipelines"
  fi
fi
if [[ "$PUSHED_TO_ORIGIN" = true ]]; then
  echo "  ✓ Pushed to origin: ${CATALOG_REPO}/${CATALOG_BRANCH}"
fi
if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
  echo "  ✓ PR to $CATALOG_BRANCH: ${PR_URL}"
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
  echo "Branch: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
  echo ""
  echo "Verify: OCP pipelines and catalog images"
elif [[ "$PR_CREATED" = true || "$CLEANUP_PR_CREATED" = true ]]; then
  PR_COUNT=0
  if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
    PR_COUNT=$((PR_COUNT + 1))
    echo "${PR_COUNT}. Review and merge PR to $CATALOG_BRANCH:"
    echo "   ${PR_URL}"
  fi
  if [[ "$CLEANUP_PR_CREATED" = true && -n "$CLEANUP_PR_URL" ]]; then
    PR_COUNT=$((PR_COUNT + 1))
    echo "${PR_COUNT}. Review and merge cleanup PR to $CLEANUP_TARGET_BRANCH:"
    echo "   ${CLEANUP_PR_URL}"
  fi
  echo ""
  echo "After merge: Verify OCP pipelines and catalog images"
else
  echo "Repository: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
fi
echo ""
echo "$SEPARATOR_LINE"
if [[ "$PUSHED_TO_ORIGIN" = true || "$PR_CREATED" = true ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES" >&2
fi
echo "$SEPARATOR_LINE"
