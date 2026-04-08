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
fi

# Constants for repeated patterns
readonly NULL_PR_VALUE='null|null'
readonly SEPARATOR_LINE='================================================'

# PR status constants
readonly PR_STATUS_NONE='none'
readonly PR_STATUS_CREATED='created'
readonly PR_STATUS_UPDATED='updated'
readonly PR_STATUS_EXISTS='exists'
readonly PR_STATUS_SKIPPED='skipped'
readonly PR_STATUS_PUSHED='pushed'
readonly PR_STATUS_FAILED='failed'

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

# Reuse existing repository or clone new one
if [[ -d "$REPO_PATH/.git" ]]; then
  echo "📂 Repository already exists, updating..."
  cd "$REPO_PATH"

  # Clean any local changes
  git reset --hard HEAD >/dev/null 2>&1 || true
  git clean -fd >/dev/null 2>&1 || true

  # Fetch latest from origin
  echo "🔄 Fetching latest changes from origin..."
  git fetch origin --depth=1 --progress 2>&1 | grep -E "Receiving|Resolving|Fetching" || true
  echo "   ✅ Repository updated"
else
  echo "📥 Cloning $CATALOG_REPO (--depth=1 for faster clone)..."
  git clone --depth=1 --single-branch --branch main --progress "https://github.com/$CATALOG_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
  if [[ ! -d "$REPO_PATH/.git" ]]; then
    echo "❌ Failed to clone $CATALOG_REPO" >&2
    exit 1
  fi
  echo "✅ Cloned successfully"
  cd "$REPO_PATH"
fi

# Setup user's fork remote (if not already added)
FORK_REPO="git@github.com:${GITHUB_USER}/multicluster-global-hub-operator-catalog.git"
if ! git remote | grep -q "^fork$"; then
  git remote add fork "$FORK_REPO" 2>/dev/null || true
fi

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/multicluster-global-hub-operator-catalog"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/multicluster-global-hub-operator-catalog" >&2
  echo "   Note: Some PRs will require manual creation if fork doesn't exist"
fi

# Note: For target release branch, we push directly to upstream (private repo CI requirement)
# For main and cleanup PRs, we still use fork if available
echo "   ℹ️  Target release PR will use upstream branch (private repo)"

# Step 0: Check and create OCP version directories on main branch
echo ""
echo "📍 Step 0: Checking OCP version directories on main branch..."
echo "   OCP range: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"

# Ensure we're on main branch
git checkout main >/dev/null 2>&1

CATALOG_DIR_CREATED=false
CATALOG_JSON_PATH="catalog/multicluster-global-hub-operator-rh/catalog.json"
MISSING_OCP_VERSIONS=()
MAIN_PR_STATUS="$PR_STATUS_NONE"
MAIN_PR_URL=""

# Check which OCP versions are missing
for ((ocp_ver=(OCP_MIN%100); ocp_ver<=(OCP_MAX%100); ocp_ver++)); do
  OCP_VERSION_DIR="v4.${ocp_ver}"
  FULL_CATALOG_PATH="${OCP_VERSION_DIR}/${CATALOG_JSON_PATH}"

  if [[ ! -f "$FULL_CATALOG_PATH" ]]; then
    echo "   ⚠️  Missing: $FULL_CATALOG_PATH"
    MISSING_OCP_VERSIONS+=("4.${ocp_ver}")
    CATALOG_DIR_CREATED=true
  else
    echo "   ✓ Found: $FULL_CATALOG_PATH"
  fi
done

# If we need to create directories, do it on a new branch and create PR
if [[ "$CATALOG_DIR_CREATED" = true ]]; then
  echo ""
  echo "   Creating OCP version directories: ${MISSING_OCP_VERSIONS[*]}"

  # Create a temporary branch from origin/main (fixed name for PR deduplication)
  MAIN_PR_BRANCH="add-ocp-dirs-${RELEASE_BRANCH}"

  # Check if branch already exists locally and delete it
  if git show-ref --verify --quiet "refs/heads/$MAIN_PR_BRANCH"; then
    git branch -D "$MAIN_PR_BRANCH" 2>/dev/null || true
  fi

  git checkout -b "$MAIN_PR_BRANCH" origin/main >/dev/null 2>&1
  echo "   ✓ Created branch $MAIN_PR_BRANCH from origin/main"

  # Create the missing directories and files
  for ((ocp_ver=(OCP_MIN%100); ocp_ver<=(OCP_MAX%100); ocp_ver++)); do
    OCP_VERSION_DIR="v4.${ocp_ver}"
    FULL_CATALOG_PATH="${OCP_VERSION_DIR}/${CATALOG_JSON_PATH}"

    if [[ ! -f "$FULL_CATALOG_PATH" ]]; then
      # Create directory structure
      mkdir -p "${OCP_VERSION_DIR}/catalog/multicluster-global-hub-operator-rh"

      # Create empty catalog.json file (must be completely empty, not {})
      : > "$FULL_CATALOG_PATH"

      echo "   ✅ Created $FULL_CATALOG_PATH"
    fi
  done

  # Commit the changes
  echo ""
  echo "   Committing new OCP version directories..."
  git add v4.*/

  CATALOG_COMMIT_MSG="Add OCP version directories for release ${RELEASE_BRANCH}

- Add empty catalog.json for OCP 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
- Prepare catalog structure for Global Hub ${GH_VERSION} release

Related: ${RELEASE_BRANCH}"

  git commit --signoff -m "$CATALOG_COMMIT_MSG"
  echo "   ✅ Changes committed"

  # Push to upstream main if in CREATE_BRANCHES mode, otherwise create PR
  if [[ "$CREATE_BRANCHES" = "true" ]]; then
    echo "   Pushing changes directly to origin/main..."
    if git push origin "$MAIN_PR_BRANCH":main 2>&1; then
      echo "   ✅ Changes pushed to origin/main"
      MAIN_PR_STATUS="$PR_STATUS_PUSHED"
    else
      echo "   ⚠️  Failed to push to origin/main" >&2
      echo "   You may need to create a PR manually for these changes"
      MAIN_PR_STATUS="$PR_STATUS_FAILED"
    fi
  else
    # UPDATE mode - create PR to upstream main
    echo "   Creating PR to upstream main..."

    if [[ "$FORK_EXISTS" = false ]]; then
      echo "   ⚠️  Cannot create PR - fork does not exist" >&2
      echo "   Please fork ${CATALOG_REPO} to enable PR creation"
    else
      # Check if PR already exists before pushing (search by title to catch PRs with different branch names)
      echo "   Checking for existing PR..."
      PR_TITLE="Add OCP version directories for ${RELEASE_BRANCH}"

      EXISTING_MAIN_PR=$(gh pr list \
        --repo "${CATALOG_REPO}" \
        --base main \
        --state open \
        --search "\"${PR_TITLE}\" in:title author:${GITHUB_USER}" \
        --json number,url,headRefName \
        --jq '.[0] | select(. != null) | "\(.url)|\(.headRefName)"' 2>/dev/null || echo "")

      if [[ -n "$EXISTING_MAIN_PR" ]]; then
        MAIN_PR_URL=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f1)
        EXISTING_BRANCH=$(echo "$EXISTING_MAIN_PR" | cut -d'|' -f2)

        echo "   ℹ️  PR already exists: $MAIN_PR_URL"
        echo "   Existing branch: ${GITHUB_USER}:${EXISTING_BRANCH}"

        # Always push updates to the existing PR's branch (even if branch name is different)
        echo "   Pushing updates to existing PR branch: $EXISTING_BRANCH..."
        if git push -f fork "$MAIN_PR_BRANCH:$EXISTING_BRANCH" 2>&1; then
          echo "   ✅ PR updated with latest changes: $MAIN_PR_URL"
          MAIN_PR_STATUS="$PR_STATUS_UPDATED"
        else
          echo "   ⚠️  Failed to push updates to fork" >&2
          MAIN_PR_STATUS="$PR_STATUS_EXISTS"
        fi
      else
        # No existing PR, create a new one
        echo "   Pushing $MAIN_PR_BRANCH to fork..."
        if git push -f fork "$MAIN_PR_BRANCH" 2>&1; then
          echo "   ✅ Branch pushed to fork"

          # Create PR to upstream main
          MAIN_PR_BODY="Add OCP version directories for ${RELEASE_BRANCH} release

## Changes

- Add empty \`catalog.json\` for OCP versions: ${MISSING_OCP_VERSIONS[*]}
- Prepare catalog structure for Global Hub ${GH_VERSION} release

## Directory Structure

\`\`\`
v4.$((OCP_MIN%100))/catalog/multicluster-global-hub-operator-rh/catalog.json
v4.$((OCP_MIN%100+1))/catalog/multicluster-global-hub-operator-rh/catalog.json
...
v4.$((OCP_MAX%100))/catalog/multicluster-global-hub-operator-rh/catalog.json
\`\`\`

## Related

- ACM: ${RELEASE_BRANCH}
- Global Hub: ${GH_VERSION}"

          MAIN_PR_URL=$(gh pr create \
            --repo "${CATALOG_REPO}" \
            --base main \
            --head "${GITHUB_USER}:${MAIN_PR_BRANCH}" \
            --title "Add OCP version directories for ${RELEASE_BRANCH}" \
            --body "$MAIN_PR_BODY" 2>&1)

          if [[ "$MAIN_PR_URL" =~ ^https:// ]]; then
            echo "   ✅ PR created to upstream main: $MAIN_PR_URL"
            MAIN_PR_STATUS="$PR_STATUS_CREATED"
          else
            echo "   ⚠️  Failed to create PR" >&2
            echo "   Reason: $MAIN_PR_URL"
            MAIN_PR_STATUS="$PR_STATUS_FAILED"
          fi
        else
          echo "   ⚠️  Failed to push branch to fork" >&2
        fi
      fi
    fi
  fi

  # Return to main branch
  git checkout main >/dev/null 2>&1
else
  echo "   ✓ All required OCP version directories exist"
  MAIN_PR_STATUS="$PR_STATUS_SKIPPED"
fi

echo ""

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


echo ""

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
CATALOG_PR_STATUS="$PR_STATUS_NONE"  # none, created, updated, exists, skipped, pushed
CATALOG_PR_URL=""
CLEANUP_PR_URL=""

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

    if [[ -f "$NEW_PR_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PR_PIPELINE"
      echo "   Skipping creation (may have been created by another PR or previous run)"
    elif [[ -f "$LATEST_PR_PIPELINE" ]]; then
      cp "$LATEST_PR_PIPELINE" "$NEW_PR_PIPELINE"
      # Update all OCP version references (v420 -> v421, v4.20 -> v4.21, catalog-420 -> catalog-421)
      sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/v4\.$((PREV_OCP_MAX%100))/v4.${NEW_OCP_VER}/g" "$NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/catalog-4$((PREV_OCP_MAX%100))-/catalog-4${NEW_OCP_VER}-/g" "$NEW_PR_PIPELINE"
      # Update catalog tag (globalhub-1-6 -> globalhub-1-7)
      sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR_PIPELINE"
      # Update branch references (release-1.6 -> release-1.7)
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Created $NEW_PR_PIPELINE"
    fi

    # Copy and update push pipeline for new OCP version
    LATEST_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4$((PREV_OCP_MAX%100))-${PREV_CATALOG_TAG}-push.yaml"
    NEW_PUSH_PIPELINE=".tekton/multicluster-global-hub-operator-catalog-v4${NEW_OCP_VER}-${CATALOG_TAG}-push.yaml"

    if [[ -f "$NEW_PUSH_PIPELINE" ]]; then
      echo "   ℹ️  Pipeline already exists: $NEW_PUSH_PIPELINE"
      echo "   Skipping creation (may have been created by another PR or previous run)"
    elif [[ -f "$LATEST_PUSH_PIPELINE" ]]; then
      cp "$LATEST_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
      # Update all OCP version references (v420 -> v421, v4.20 -> v4.21, catalog-420 -> catalog-421)
      sed "${SED_INPLACE[@]}" "s/v4$((PREV_OCP_MAX%100))/v4${NEW_OCP_VER}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/v4\.$((PREV_OCP_MAX%100))/v4.${NEW_OCP_VER}/g" "$NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/catalog-4$((PREV_OCP_MAX%100))-/catalog-4${NEW_OCP_VER}-/g" "$NEW_PUSH_PIPELINE"
      # Update catalog tag (globalhub-1-6 -> globalhub-1-7)
      sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH_PIPELINE"
      # Update branch references (release-1.6 -> release-1.7)
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH_PIPELINE"
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
      # Same tag - pipeline should already exist
      if [[ -f "$NEW_PR" ]]; then
        echo "   ℹ️  Pipeline already exists: v4${ocp_ver} pull-request"
        echo "   Skipping modification (may have been updated by other PRs)"
      fi
      if [[ -f "$NEW_PUSH" ]]; then
        echo "   ℹ️  Pipeline already exists: v4${ocp_ver} push"
        echo "   Skipping modification (may have been updated by other PRs)"
      fi
    else
      # Different tags - check if new pipeline already exists first
      if [[ -f "$NEW_PR" ]]; then
        echo "   ℹ️  Pipeline already exists: v4${ocp_ver} pull-request"
        echo "   Skipping modification (may have been created by another PR or previous run)"
      elif [[ -f "$OLD_PR" ]]; then
        git mv "$OLD_PR" "$NEW_PR" 2>/dev/null || cp "$OLD_PR" "$NEW_PR"
        # Update catalog tag (globalhub-1-6 -> globalhub-1-7)
        sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PR"
        # Update branch references (release-1.6 -> release-1.7)
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PR"
        echo "   ✅ Updated v4${ocp_ver} pull-request pipeline"
      fi

      if [[ -f "$NEW_PUSH" ]]; then
        echo "   ℹ️  Pipeline already exists: v4${ocp_ver} push"
        echo "   Skipping modification (may have been created by another PR or previous run)"
      elif [[ -f "$OLD_PUSH" ]]; then
        git mv "$OLD_PUSH" "$NEW_PUSH" 2>/dev/null || cp "$OLD_PUSH" "$NEW_PUSH"
        # Update catalog tag (globalhub-1-6 -> globalhub-1-7)
        sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_PUSH"
        # Update branch references (release-1.6 -> release-1.7)
        sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$NEW_PUSH"
        echo "   ✅ Updated v4${ocp_ver} push pipeline"
      fi
    fi
  done

  # Remove all tekton pipelines for OCP versions below OCP_MIN
  echo ""
  echo "   Cleaning up outdated OCP tekton pipelines (below 4.$((OCP_MIN%100)))..."
  CLEANED_PIPELINES=false
  for old_pipeline in .tekton/multicluster-global-hub-operator-catalog-v4*; do
    [[ -f "$old_pipeline" ]] || continue
    # Extract OCP version number from filename (e.g., v416 -> 16, v417 -> 17)
    pipeline_ocp_ver=$(echo "$old_pipeline" | grep -oE 'v4[0-9]+' | head -1 | sed 's/v4//')
    if [[ -n "$pipeline_ocp_ver" ]] && (( pipeline_ocp_ver < (OCP_MIN%100) )); then
      git rm "$old_pipeline" 2>/dev/null || rm "$old_pipeline"
      echo "   ✅ Removed outdated pipeline: $(basename "$old_pipeline")"
      CLEANED_PIPELINES=true
    fi
  done
  if [[ "$CLEANED_PIPELINES" = false ]]; then
    echo "   ✓ No outdated tekton pipelines found"
  fi

  # Update Containerfile.catalog for OCP versions (only if OCP range changed)
  if [[ "$PREV_OCP_MAX" != "$OCP_MAX" ]]; then
    echo ""
    echo "   Updating Containerfile.catalog files..."

    # Remove all OCP version directories below OCP_MIN
    for old_dir in v4.*/; do
      [[ -d "$old_dir" ]] || continue
      old_ver="${old_dir#v4.}"
      old_ver="${old_ver%/}"
      if [[ "$old_ver" =~ ^[0-9]+$ ]] && (( old_ver < (OCP_MIN%100) )); then
        git rm -r "$old_dir" 2>/dev/null || rm -rf "$old_dir"
        echo "   ✅ Removed old OCP directory: ${old_dir%/}"
      fi
    done

    # Create new OCP version directory and Containerfile.catalog
    NEW_OCP_DIR="v4.${NEW_OCP_VER}"
    PREV_OCP_DIR="v4.$((PREV_OCP_MAX%100))"
    PREV_CONTAINERFILE="${PREV_OCP_DIR}/Containerfile.catalog"
    NEW_CONTAINERFILE="${NEW_OCP_DIR}/Containerfile.catalog"

    if [[ -f "$NEW_CONTAINERFILE" ]]; then
      echo "   ℹ️  Containerfile already exists: $NEW_CONTAINERFILE"
    elif [[ -f "$PREV_CONTAINERFILE" ]]; then
      mkdir -p "$NEW_OCP_DIR"
      cp "$PREV_CONTAINERFILE" "$NEW_CONTAINERFILE"

      sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$NEW_CONTAINERFILE"
      sed "${SED_INPLACE[@]}" "s|configs/v4.$((PREV_OCP_MAX%100))/|configs/v4.${NEW_OCP_VER}/|g" "$NEW_CONTAINERFILE"
      sed "${SED_INPLACE[@]}" "s|ose-operator-registry-rhel9:v4.$((PREV_OCP_MAX%100))|ose-operator-registry-rhel9:v4.${NEW_OCP_VER}|g" "$NEW_CONTAINERFILE"
      echo "   ✅ Created new Containerfile: $NEW_CONTAINERFILE"
    else
      echo "   ⚠️  Previous Containerfile not found: $PREV_CONTAINERFILE" >&2
    fi

    if [[ -f "$NEW_CONTAINERFILE" ]]; then
      if grep -qF 'filter_catalog.py' "$NEW_CONTAINERFILE"; then
        sed "${SED_INPLACE[@]}" '/# Copy filter script/d' "$NEW_CONTAINERFILE"
        sed "${SED_INPLACE[@]}" '/COPY filter_catalog.py/d' "$NEW_CONTAINERFILE"
        sed "${SED_INPLACE[@]}" '/# remove the existing entries in catalog.json file/d' "$NEW_CONTAINERFILE"
        sed "${SED_INPLACE[@]}" '/RUN python3 \/filter_catalog.py/d' "$NEW_CONTAINERFILE"
        echo "   ✅ Normalized $NEW_CONTAINERFILE (filter_catalog.py lines removed)"
      fi
      git add "$NEW_CONTAINERFILE"
    fi
  fi

  # Update existing OCP versions' Containerfile.catalog files
  if [[ -n "$PREV_CATALOG_TAG" ]]; then
    echo ""
    echo "   Updating existing OCP versions' Containerfile.catalog files..."

    # Update all Containerfile.catalog files in the OCP range
    for ((ocp_ver=OCP_MIN%100; ocp_ver<=OCP_MAX%100; ocp_ver++)); do
      OCP_DIR="v4.${ocp_ver}"
      CONTAINERFILE="${OCP_DIR}/Containerfile.catalog"

      if [[ -f "$CONTAINERFILE" ]]; then
        # Check if file contains the previous catalog tag
        if grep -q "$PREV_CATALOG_TAG" "$CONTAINERFILE"; then
          # Update bundle reference: globalhub-1-6 -> globalhub-1-7
          sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$CONTAINERFILE"
          git add "$CONTAINERFILE"
          echo "   ✅ Updated bundle reference in $CONTAINERFILE: ${PREV_CATALOG_TAG} -> ${CATALOG_TAG}"
        else
          echo "   ℹ️  $CONTAINERFILE already has correct bundle reference"
        fi
      fi
    done
  fi
fi

# Step 2.5: Update catalog-template-current.json
echo ""
echo "📍 Step 2.5: Updating catalog-template-current.json..."

CATALOG_TEMPLATE_FILE="catalog-template-current.json"
if [[ -f "$CATALOG_TEMPLATE_FILE" && -n "$PREV_CATALOG_VERSION" ]]; then
  echo "   Updating catalog template for Global Hub ${GH_VERSION_SHORT}"

  # 1. Update defaultChannel: "release-1.6" -> "release-1.7"
  sed "${SED_INPLACE[@]}" "s/\"defaultChannel\": \"release-${PREV_CATALOG_VERSION}\"/\"defaultChannel\": \"${CATALOG_BRANCH}\"/g" "$CATALOG_TEMPLATE_FILE"

  # 2. Update operator version in entries name: v1.6.0 -> v1.7.0
  sed "${SED_INPLACE[@]}" "s/multicluster-global-hub-operator-rh\.v${PREV_CATALOG_VERSION}\.0/multicluster-global-hub-operator-rh.v${GH_VERSION_SHORT}.0/g" "$CATALOG_TEMPLATE_FILE"

  # 3. Update skipRange: ">=1.5.0 <1.6.0" -> ">=1.6.0 <1.7.0"
  # Calculate previous version for skipRange (current - 1)
  PREV_MINOR="${PREV_CATALOG_VERSION#1.}"
  SKIP_PREV_MINOR=$((PREV_MINOR - 1))
  sed "${SED_INPLACE[@]}" "s/\\\\u003e=1\.${SKIP_PREV_MINOR}\.0 \\\\u003c1\.${PREV_MINOR}\.0/\\\\u003e=1.${PREV_MINOR}.0 \\\\u003c1.${GH_VERSION_SHORT#1.}.0/g" "$CATALOG_TEMPLATE_FILE"

  # 4. Update channel name: "release-1.6" -> "release-1.7"
  sed "${SED_INPLACE[@]}" "s/\"name\": \"release-${PREV_CATALOG_VERSION}\"/\"name\": \"${CATALOG_BRANCH}\"/g" "$CATALOG_TEMPLATE_FILE"

  # 5. Update bundle image reference: globalhub-1-6 -> globalhub-1-7
  sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$CATALOG_TEMPLATE_FILE"

  echo "   ✅ Updated $CATALOG_TEMPLATE_FILE"
  echo "      - defaultChannel: release-${PREV_CATALOG_VERSION} → ${CATALOG_BRANCH}"
  echo "      - operator version: v${PREV_CATALOG_VERSION}.0 → v${GH_VERSION_SHORT}.0"
  echo "      - skipRange: >=1.${SKIP_PREV_MINOR}.0 <1.${PREV_MINOR}.0 → >=1.${PREV_MINOR}.0 <1.${GH_VERSION_SHORT#1.}.0"
  echo "      - channel name: release-${PREV_CATALOG_VERSION} → ${CATALOG_BRANCH}"
  echo "      - bundle: ${PREV_CATALOG_TAG} → ${CATALOG_TAG}"
elif [[ ! -f "$CATALOG_TEMPLATE_FILE" ]]; then
  echo "   ⚠️  File not found: $CATALOG_TEMPLATE_FILE" >&2
fi

# Step 2.6: Update filter_catalog.py
echo ""
echo "📍 Step 2.6: Updating filter_catalog.py..."

FILTER_CATALOG_FILE="filter_catalog.py"
if [[ -f "$FILTER_CATALOG_FILE" && -n "$PREV_CATALOG_VERSION" ]]; then
  # Calculate the version before the previous version (e.g., for 1.7, prev is 1.6, skip_prev is 1.5)
  PREV_MINOR="${PREV_CATALOG_VERSION#1.}"
  SKIP_PREV_MINOR=$((PREV_MINOR - 1))

  echo "   Updating filter_catalog.py defaultChannel"
  echo "   Changing: release-1.${SKIP_PREV_MINOR}"
  echo "   To:       release-${PREV_CATALOG_VERSION}"

  # Update the defaultChannel in filter_catalog.py from previous-previous to previous version
  # Example: For release-1.7, change "release-1.5" to "release-1.6"
  sed "${SED_INPLACE[@]}" "s/\"release-1\.${SKIP_PREV_MINOR}\"/\"release-${PREV_CATALOG_VERSION}\"/g" "$FILTER_CATALOG_FILE"

  echo "   ✅ Updated $FILTER_CATALOG_FILE"
  echo "      - defaultChannel: release-1.${SKIP_PREV_MINOR} → release-${PREV_CATALOG_VERSION}"
elif [[ ! -f "$FILTER_CATALOG_FILE" ]]; then
  echo "   ⚠️  File not found: $FILTER_CATALOG_FILE" >&2
fi

# Step 2.7: Update README.md
echo ""
echo "📍 Step 2.7: Updating README.md..."

README_FILE="README.md"
if [[ -f "$README_FILE" && -n "$PREV_CATALOG_TAG" ]]; then
  README_UPDATED=false
  if grep -qF "$PREV_CATALOG_TAG" "$README_FILE"; then
    sed "${SED_INPLACE[@]}" "s/${PREV_CATALOG_TAG}/${CATALOG_TAG}/g" "$README_FILE"
    README_UPDATED=true
    echo "      - ${PREV_CATALOG_TAG} → ${CATALOG_TAG}"
  fi
  if grep -qF "$BASE_BRANCH" "$README_FILE"; then
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${CATALOG_BRANCH}/g" "$README_FILE"
    README_UPDATED=true
    echo "      - ${BASE_BRANCH} → ${CATALOG_BRANCH}"
  fi
  if [[ "$README_UPDATED" = true ]]; then
    echo "   ✅ Updated $README_FILE"
  else
    echo "   ℹ️  $README_FILE already has correct version references"
  fi
elif [[ ! -f "$README_FILE" ]]; then
  echo "   ⚠️  File not found: $README_FILE" >&2
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
- Update catalog-template-current.json (release channel and operator version)
- Update filter_catalog.py defaultChannel to ${BASE_BRANCH}
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Add OCP 4.${NEW_OCP_VER} Containerfile.catalog
- Remove OCP 4.${OLD_OCP_VER} pipelines and directory
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines
- Update all Containerfile.catalog files with new bundle reference (${PREV_CATALOG_TAG} -> ${CATALOG_TAG})
- Update README.md image references to ${CATALOG_TAG}

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
        CATALOG_PR_STATUS="$PR_STATUS_PUSHED"
      else
        echo "   ❌ Failed to push branch to origin" >&2
        exit 1
      fi
    else
      # Branch exists or UPDATE mode - create PR to update it
      # For target release branch, push directly to upstream (private repo CI requirement)
      echo "   Creating PR to update $CATALOG_BRANCH (using upstream branch)..."

      # Use fixed branch name for PR deduplication
      PR_BRANCH="${CATALOG_BRANCH}-update"

      # Check if branch already exists locally and delete it
      if git show-ref --verify --quiet "refs/heads/$PR_BRANCH"; then
        git branch -D "$PR_BRANCH" 2>/dev/null || true
      fi

      git checkout -b "$PR_BRANCH"

      # Check if PR already exists (search by title)
      echo "   Checking for existing PR to $CATALOG_BRANCH..."
      PR_TITLE="Update ${CATALOG_BRANCH} catalog configuration"

      EXISTING_PR=$(gh pr list \
        --repo "${CATALOG_REPO}" \
        --base "$CATALOG_BRANCH" \
        --state open \
        --search "\"${PR_TITLE}\" in:title" \
        --json number,url,headRefName,headRepositoryOwner \
        --jq '.[0] | select(. != null) | "\(.number)|\(.url)|\(.headRefName)|\(.headRepositoryOwner.login)"' 2>/dev/null || echo "")

      if [[ -n "$EXISTING_PR" ]]; then
        PR_NUMBER=$(echo "$EXISTING_PR" | cut -d'|' -f1)
        CATALOG_PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
        EXISTING_BRANCH=$(echo "$EXISTING_PR" | cut -d'|' -f3)
        HEAD_REPO_OWNER=$(echo "$EXISTING_PR" | cut -d'|' -f4)

        echo "   ℹ️  Found existing PR #${PR_NUMBER}: $CATALOG_PR_URL"
        echo "   Branch: ${EXISTING_BRANCH} (owner: ${HEAD_REPO_OWNER})"

        # Check if PR is from fork or upstream
        CATALOG_REPO_OWNER=$(echo "$CATALOG_REPO" | cut -d'/' -f1)
        if [[ "$HEAD_REPO_OWNER" != "$CATALOG_REPO_OWNER" ]]; then
          # PR is from fork, close it and create new one from upstream
          echo "   ⚠️  Existing PR is from fork (${HEAD_REPO_OWNER})"
          echo "   Closing old PR and creating new one from upstream..."

          gh pr close "$PR_NUMBER" --repo "${CATALOG_REPO}" --comment "Closing this PR as it was created from a fork. Creating a new PR from upstream branch to fix CI issues in private repo." 2>&1 || true

          # Continue to create new PR below
          EXISTING_PR=""
        else
          # PR is from upstream, update it
          echo "   ✅ PR is from upstream, updating existing PR..."
          echo "   Pushing updates to existing PR branch: $EXISTING_BRANCH..."
          if git push -f origin "HEAD:$EXISTING_BRANCH" 2>&1; then
            echo "   ✅ PR updated with latest changes: $CATALOG_PR_URL"
            CATALOG_PR_STATUS="$PR_STATUS_UPDATED"
          else
            echo "   ⚠️  Failed to push updates to upstream" >&2
            CATALOG_PR_STATUS="$PR_STATUS_EXISTS"
          fi
        fi
      fi

      if [[ -z "$EXISTING_PR" ]]; then
        # No existing PR, create a new one
        echo "   Pushing $PR_BRANCH to upstream..."
        if git push -f origin "$PR_BRANCH" 2>&1; then
          echo "   ✅ PR branch pushed to upstream"

          PR_BODY="Update ${CATALOG_BRANCH} catalog configuration

## Changes

- Update images-mirror-set.yaml to use \`${CATALOG_TAG}\`
- Update catalog-template-current.json (release channel and operator version)
- Update filter_catalog.py defaultChannel to \`${BASE_BRANCH}\`
- Add OCP 4.${NEW_OCP_VER} pipelines (pull-request and push)
- Remove OCP 4.${OLD_OCP_VER} pipelines
- Update existing OCP 4.$((OCP_MIN%100))-4.$((OCP_MAX-1%100)) pipelines
- Update all Containerfile.catalog files with new bundle reference (\`${PREV_CATALOG_TAG}\` → \`${CATALOG_TAG}\`)

## Version Mapping

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: ${GH_VERSION}
- **Catalog branch**: ${CATALOG_BRANCH}
- **Supported OCP**: 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))
- **Previous release**: ${BASE_BRANCH}"

          PR_CREATE_OUTPUT=$(gh pr create --base "$CATALOG_BRANCH" --head "$PR_BRANCH" \
            --title "Update ${CATALOG_BRANCH} catalog configuration" \
            --body "$PR_BODY" \
            --repo "$CATALOG_REPO" 2>&1) || true

          if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
            CATALOG_PR_URL="$PR_CREATE_OUTPUT"
            echo "   ✅ PR created: $CATALOG_PR_URL"
            CATALOG_PR_STATUS="$PR_STATUS_CREATED"
          elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
            CATALOG_PR_URL="${BASH_REMATCH[1]}"
            echo "   ✅ PR created: $CATALOG_PR_URL"
            CATALOG_PR_STATUS="$PR_STATUS_CREATED"
          else
            echo "   ⚠️  Failed to create PR" >&2
            echo "   Reason: $PR_CREATE_OUTPUT"
            CATALOG_PR_STATUS="$PR_STATUS_FAILED"
          fi
        else
          echo "   ❌ Failed to push PR branch to upstream" >&2
          CATALOG_PR_STATUS="$PR_STATUS_FAILED"
        fi
      fi
    fi
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

# Step 0: OCP directories on main
case "$MAIN_PR_STATUS" in
  "$PR_STATUS_SKIPPED")
    echo "  ✓ OCP directories: All exist (4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100)))"
    ;;
  "$PR_STATUS_CREATED")
    echo "  ✓ OCP directories: Created missing directories (${MISSING_OCP_VERSIONS[*]})"
    echo "  ✓ Main branch PR: Created - $MAIN_PR_URL"
    ;;
  "$PR_STATUS_UPDATED")
    echo "  ✓ OCP directories: Updated missing directories (${MISSING_OCP_VERSIONS[*]})"
    echo "  ✓ Main branch PR: Updated - $MAIN_PR_URL"
    ;;
  "$PR_STATUS_EXISTS")
    echo "  ✓ OCP directories: PR already exists (no changes) - $MAIN_PR_URL"
    ;;
  "$PR_STATUS_PUSHED")
    echo "  ✓ OCP directories: Pushed to main (${MISSING_OCP_VERSIONS[*]})"
    ;;
  "$PR_STATUS_FAILED")
    echo "  ⚠️  OCP directories: Failed to create/push" >&2
    ;;
  *)
    echo "  ⚠️  OCP directories: Unknown status ($MAIN_PR_STATUS)" >&2
    ;;
esac

# Catalog branch tasks
echo "  ✓ Catalog branch: $CATALOG_BRANCH (from $BASE_BRANCH)"
if [[ "$CHANGES_COMMITTED" = true ]]; then
  echo "  ✓ Updated images-mirror-set.yaml to ${CATALOG_TAG}"
  if [[ -n "$PREV_CATALOG_TAG" ]]; then
    echo "  ✓ Updated catalog-template-current.json"
    echo "  ✓ Updated filter_catalog.py defaultChannel to ${BASE_BRANCH}"
    if [[ "$PREV_OCP_MAX" != "$OCP_MAX" ]]; then
      echo "  ✓ Added OCP 4.${NEW_OCP_VER} pipelines and Containerfile"
      echo "  ✓ Removed OCP 4.${OLD_OCP_VER} pipelines and directory"
    fi
    echo "  ✓ Updated OCP 4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100-1)) pipelines"
  fi
fi

# Catalog PR status
case "$CATALOG_PR_STATUS" in
  "$PR_STATUS_PUSHED")
    echo "  ✓ Catalog branch: Pushed to origin - ${CATALOG_REPO}/${CATALOG_BRANCH}"
    ;;
  "$PR_STATUS_CREATED")
    echo "  ✓ Catalog branch PR: Created - $CATALOG_PR_URL"
    ;;
  "$PR_STATUS_UPDATED")
    echo "  ✓ Catalog branch PR: Updated - $CATALOG_PR_URL"
    ;;
  "$PR_STATUS_EXISTS")
    echo "  ✓ Catalog branch PR: Exists (no changes) - $CATALOG_PR_URL"
    ;;
  *)
    echo "  ⚠️  Catalog branch: Unknown status ($CATALOG_PR_STATUS)" >&2
    ;;
esac

echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"

PR_COUNT=0

# Step 0: Main branch PR
if [[ "$MAIN_PR_STATUS" = "$PR_STATUS_CREATED" || "$MAIN_PR_STATUS" = "$PR_STATUS_UPDATED" || "$MAIN_PR_STATUS" = "$PR_STATUS_EXISTS" ]]; then
  PR_COUNT=$((PR_COUNT + 1))
  case "$MAIN_PR_STATUS" in
    "$PR_STATUS_CREATED")
      echo "${PR_COUNT}. Review and merge PR to main (OCP directories):"
      ;;
    "$PR_STATUS_UPDATED")
      echo "${PR_COUNT}. Review updated PR to main (OCP directories):"
      ;;
    "$PR_STATUS_EXISTS")
      echo "${PR_COUNT}. Review existing PR to main (OCP directories):"
      ;;
    *)
      echo "${PR_COUNT}. Review PR to main (unknown status: $MAIN_PR_STATUS):" >&2
      ;;
  esac
  echo "   ${MAIN_PR_URL}"
fi

# Catalog branch PR
if [[ "$CATALOG_PR_STATUS" = "$PR_STATUS_PUSHED" ]]; then
  if [[ $PR_COUNT -eq 0 ]]; then
    echo "✅ Branch pushed to origin successfully"
    echo ""
    echo "Branch: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
    echo ""
    echo "Verify: OCP pipelines and catalog images"
  else
    echo ""
    echo "✅ Catalog branch pushed to origin"
    echo "   Branch: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
  fi
elif [[ "$CATALOG_PR_STATUS" = "$PR_STATUS_CREATED" || "$CATALOG_PR_STATUS" = "$PR_STATUS_UPDATED" || "$CATALOG_PR_STATUS" = "$PR_STATUS_EXISTS" ]]; then
  PR_COUNT=$((PR_COUNT + 1))
  case "$CATALOG_PR_STATUS" in
    "$PR_STATUS_CREATED")
      echo "${PR_COUNT}. Review and merge PR to $CATALOG_BRANCH:"
      ;;
    "$PR_STATUS_UPDATED")
      echo "${PR_COUNT}. Review updated PR to $CATALOG_BRANCH:"
      ;;
    "$PR_STATUS_EXISTS")
      echo "${PR_COUNT}. Review existing PR to $CATALOG_BRANCH:"
      ;;
    *)
      echo "${PR_COUNT}. Review PR to $CATALOG_BRANCH (status: $CATALOG_PR_STATUS):"
      ;;
  esac
  echo "   ${CATALOG_PR_URL}"
fi

if [[ $PR_COUNT -gt 0 ]]; then
  echo ""
  echo "After merge: Verify OCP pipelines and catalog images"
elif [[ "$CATALOG_PR_STATUS" != "$PR_STATUS_PUSHED" ]]; then
  echo "Repository: https://github.com/$CATALOG_REPO/tree/$CATALOG_BRANCH"
fi
echo ""
echo "$SEPARATOR_LINE"
if [[ "$CATALOG_PR_STATUS" = "$PR_STATUS_PUSHED" || "$CATALOG_PR_STATUS" = "$PR_STATUS_CREATED" || "$CATALOG_PR_STATUS" = "$PR_STATUS_UPDATED" || "$MAIN_PR_STATUS" = "$PR_STATUS_CREATED" || "$MAIN_PR_STATUS" = "$PR_STATUS_UPDATED" ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES" >&2
fi
echo "$SEPARATOR_LINE"
