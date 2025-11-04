#!/bin/bash

set -euo pipefail

# Postgres Exporter Release Script
# Supports two modes:
#   CUT_MODE=true:  Create and push release branch directly to upstream
#   CUT_MODE=false: Update existing release branch via PR
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   POSTGRES_TAG      - Postgres tag (e.g., globalhub-1-8)
#   GITHUB_USER       - GitHub username for PR creation
#   CUT_MODE          - true: create branch, false: update via PR

# Configuration
POSTGRES_REPO="stolostron/postgres_exporter"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$POSTGRES_TAG" || -z "$GITHUB_USER" || -z "$CUT_MODE" ]]; then
  echo "❌ Error: Required environment variables not set" >&2 >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, POSTGRES_TAG, GITHUB_USER, CUT_MODE"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Postgres Exporter Release"
echo "================================================"
echo "   Mode: $([ "$CUT_MODE" = true ] && echo "CUT (create branch)" || echo "UPDATE (PR only)")"
echo "   Release: $RELEASE_BRANCH"
echo "   Postgres Tag: $POSTGRES_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/postgres_exporter"
mkdir -p "$WORK_DIR"

# Remove existing directory for clean clone
if [[ -d "$REPO_PATH" ]]; then
  echo "   Removing existing directory for clean clone..."
  rm -rf "$REPO_PATH"
fi

echo "📥 Cloning $POSTGRES_REPO..."
git clone --progress "https://github.com/$POSTGRES_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
if [[ ! -d "$REPO_PATH/.git" ]]; then
  echo "❌ Failed to clone $POSTGRES_REPO" >&2
  exit 1
fi
echo "✅ Cloned successfully"

cd "$REPO_PATH"

# Setup user's fork remote
FORK_REPO="git@github.com:${GITHUB_USER}/postgres_exporter.git"
git remote add fork "$FORK_REPO" 2>/dev/null || true

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/postgres_exporter"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/postgres_exporter"
fi

# Fetch all release branches
echo "🔄 Fetching release branches..."
git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
echo "   ✅ Release branches fetched"

# Find latest postgres release branch
LATEST_POSTGRES_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

# Check if target branch is the same as latest
if [[ "$LATEST_POSTGRES_RELEASE" = "$RELEASE_BRANCH" ]]; then
  echo "ℹ️  Target postgres branch is the latest: $RELEASE_BRANCH"
  echo ""
  echo "   https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
  echo ""
  echo "   Will verify and update if needed..."
  echo ""
fi

if [[ -z "$LATEST_POSTGRES_RELEASE" ]]; then
  echo "⚠️  No previous postgres release branch found, using main as base"
  BASE_BRANCH="main"
else
  echo "Latest postgres release detected: $LATEST_POSTGRES_RELEASE"

  # If target branch is the latest, use second-to-latest as base
  # If target branch is not the latest, use latest as base
  if [[ "$LATEST_POSTGRES_RELEASE" = "$RELEASE_BRANCH" ]]; then
    # Target is latest - get second-to-latest for base
    SECOND_TO_LATEST=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
      sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
    if [[ -n "$SECOND_TO_LATEST" && "$SECOND_TO_LATEST" != "$RELEASE_BRANCH" ]]; then
      BASE_BRANCH="$SECOND_TO_LATEST"
      echo "Target is latest release, using previous release as base: $BASE_BRANCH"
    else
      BASE_BRANCH="main"
      echo "No previous release found, using main as base"
    fi
  else
    # Target is not latest - use latest as base
    BASE_BRANCH="$LATEST_POSTGRES_RELEASE"
    echo "Target is older than latest, using latest as base: $BASE_BRANCH"
  fi
fi

# Extract previous Postgres tag info
if [[ "$BASE_BRANCH" != "main" ]]; then
  PREV_VERSION="${BASE_BRANCH#release-}"
  PREV_MINOR=$(echo "$PREV_VERSION" | cut -d. -f2)
  PREV_GH_MINOR=$((PREV_MINOR - 9))
  PREV_POSTGRES_TAG="globalhub-1-${PREV_GH_MINOR}"
  echo "Previous Postgres tag: $PREV_POSTGRES_TAG"
else
  PREV_POSTGRES_TAG=""
fi

echo ""

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
PR_CREATED=false
PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $RELEASE_BRANCH already exists on origin"
  git fetch origin "$RELEASE_BRANCH" 2>/dev/null || true
  git checkout -B "$RELEASE_BRANCH" "origin/$RELEASE_BRANCH"
else
  if [[ "$CUT_MODE" != "true" ]]; then
    echo "❌ Error: Branch $RELEASE_BRANCH does not exist on origin" >&2 >&2
    echo "   Run with CUT_MODE=true to create the branch"
    exit 1
  fi
  echo "🌿 Creating $RELEASE_BRANCH from origin/$BASE_BRANCH..."
  git branch -D "$RELEASE_BRANCH" 2>/dev/null || true
  git checkout -b "$RELEASE_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $RELEASE_BRANCH"
fi

# Step 1: Update tekton pipeline files
echo ""
echo "📍 Step 1: Updating tekton pipeline files..."

if [[ -n "$PREV_POSTGRES_TAG" ]]; then
  # Update pull-request pipeline
  OLD_PR_PIPELINE=".tekton/postgres-exporter-${PREV_POSTGRES_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/postgres-exporter-${POSTGRES_TAG}-pull-request.yaml"

  if [[ "$PREV_POSTGRES_TAG" = "$POSTGRES_TAG" ]]; then
    # Same tag - just update in place
    if [[ -f "$NEW_PR_PIPELINE" ]]; then
      echo "   ℹ️  Pull-request pipeline already exists with correct name"
      echo "   Updating references in $NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Updated $NEW_PR_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PR_PIPELINE"
    fi
  elif [[ -f "$OLD_PR_PIPELINE" ]]; then
    echo "   Renaming and updating pull-request pipeline..."
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PR_PIPELINE"
    echo "   ✅ Updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE"
  fi

  # Update push pipeline
  OLD_PUSH_PIPELINE=".tekton/postgres-exporter-${PREV_POSTGRES_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/postgres-exporter-${POSTGRES_TAG}-push.yaml"

  if [[ "$PREV_POSTGRES_TAG" = "$POSTGRES_TAG" ]]; then
    # Same tag - just update in place
    if [[ -f "$NEW_PUSH_PIPELINE" ]]; then
      echo "   ℹ️  Push pipeline already exists with correct name"
      echo "   Updating references in $NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
      echo "   ✅ Updated $NEW_PUSH_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PUSH_PIPELINE"
    fi
  elif [[ -f "$OLD_PUSH_PIPELINE" ]]; then
    echo "   Renaming and updating push pipeline..."
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
    echo "   ✅ Updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE"
  fi
else
  echo "   ⚠️  No previous postgres tag found, skipping pipeline update"
fi

# Step 2: Commit changes
echo ""
echo "📍 Step 2: Committing changes on $RELEASE_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  CHANGES_COMMITTED=false
else
  git add -A

  COMMIT_MSG="Update postgres_exporter for ${RELEASE_BRANCH} (Global Hub ${GH_VERSION})

- Rename and update pull-request pipeline for ${POSTGRES_TAG}
- Rename and update push pipeline for ${POSTGRES_TAG}
- Update branch references to ${RELEASE_BRANCH}

Corresponds to ACM ${RELEASE_BRANCH} / Global Hub ${GH_VERSION}"

  git commit --signoff -m "$COMMIT_MSG"
  echo "   ✅ Changes committed"
  CHANGES_COMMITTED=true
fi

# Step 3: Push to origin or create PR
echo ""
echo "📍 Step 3: Publishing changes..."

if [[ "$CHANGES_COMMITTED" = false ]]; then
  echo "   ℹ️  No changes to publish"
else
  # Decision: Push directly or create PR based on CUT_MODE and branch existence
  if [[ "$CUT_MODE" = "true" && "$BRANCH_EXISTS_ON_ORIGIN" = false ]]; then
    # CUT mode + branch doesn't exist - push directly
    echo "   Pushing new branch $RELEASE_BRANCH to origin..."
    if git push origin "$RELEASE_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin: $POSTGRES_REPO/$RELEASE_BRANCH"
      PUSHED_TO_ORIGIN=true
    else
      echo "   ❌ Failed to push branch to origin" >&2
      exit 1
    fi
  else
    # Branch exists or UPDATE mode - create PR to update it
    echo "   Creating PR to update $RELEASE_BRANCH..."

    # Check if PR already exists
    echo "   Checking for existing PR to $RELEASE_BRANCH..."
    EXISTING_PR=$(gh pr list \
      --repo "${POSTGRES_REPO}" \
      --base "$RELEASE_BRANCH" \
      --state open \
      --search "Update ${RELEASE_BRANCH} postgres_exporter configuration" \
      --json number,url,state \
      --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

    if [[ -n "$EXISTING_PR" && "$EXISTING_PR" != "null|null" ]]; then
      PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
      PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
      echo "   ℹ️  PR already exists (state: $PR_STATE): $PR_URL"
      PR_CREATED=true
    else
      PR_BRANCH="${RELEASE_BRANCH}-update-$(date +%s)"
      git checkout -b "$PR_BRANCH"

      if [[ "$FORK_EXISTS" = false ]]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist"
        echo "   Please fork ${POSTGRES_REPO} to enable PR creation"
        PR_CREATED=false
      else
        echo "   Pushing $PR_BRANCH to fork..."
        if git push -f fork "$PR_BRANCH" 2>&1; then
          echo "   ✅ PR branch pushed to fork"

          PR_BODY="Update ${RELEASE_BRANCH} postgres_exporter configuration

## Changes

- Rename and update pull-request pipeline for \`${POSTGRES_TAG}\`
- Rename and update push pipeline for \`${POSTGRES_TAG}\`
- Update branch references to \`${RELEASE_BRANCH}\`

## Version Mapping

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: ${GH_VERSION}
- **Postgres tag**: ${POSTGRES_TAG}
- **Previous release**: ${BASE_BRANCH}"

          PR_CREATE_OUTPUT=$(gh pr create --base "$RELEASE_BRANCH" --head "${GITHUB_USER}:$PR_BRANCH" \
            --title "Update ${RELEASE_BRANCH} postgres_exporter configuration" \
            --body "$PR_BODY" \
            --repo "$POSTGRES_REPO" 2>&1) || true

          if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
            PR_URL="$PR_CREATE_OUTPUT"
            echo "   ✅ PR created: $PR_URL"
            PR_CREATED=true
          elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
            PR_URL="${BASH_REMATCH[1]}"
            echo "   ✅ PR created: $PR_URL"
            PR_CREATED=true
          else
            echo "   ⚠️  Failed to create PR"
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

# Summary
echo ""
echo "================================================"
echo "📊 WORKFLOW SUMMARY"
echo "================================================"
echo "Release: $RELEASE_BRANCH"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Postgres branch: $RELEASE_BRANCH (from $BASE_BRANCH)"
if [[ "$CHANGES_COMMITTED" = true ]]; then
  if [[ -n "$PREV_POSTGRES_TAG" ]]; then
    echo "  ✓ Renamed tekton pipelines (${PREV_POSTGRES_TAG} → ${POSTGRES_TAG})"
  else
    echo "  ✓ Updated tekton pipelines to ${POSTGRES_TAG}"
  fi
fi
if [[ "$PUSHED_TO_ORIGIN" = true ]]; then
  echo "  ✓ Pushed to origin: ${POSTGRES_REPO}/${RELEASE_BRANCH}"
fi
if [[ "$PR_CREATED" = true && -n "$PR_URL" ]]; then
  echo "  ✓ PR to $RELEASE_BRANCH: ${PR_URL}"
fi
echo ""
echo "================================================"
echo "📝 NEXT STEPS"
echo "================================================"
if [[ "$PUSHED_TO_ORIGIN" = true ]]; then
  echo "✅ Branch pushed to origin successfully"
  echo ""
  echo "Branch: https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
  echo ""
  echo "Verify: Tekton pipelines and postgres exporter images"
elif [[ "$PR_CREATED" = true ]]; then
  echo "1. Review and merge PR to $RELEASE_BRANCH:"
  echo "   ${PR_URL}"
  echo ""
  echo "After merge: Verify tekton pipelines and postgres exporter images"
else
  echo "Repository: https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
fi
echo ""
echo "================================================"
if [[ "$PUSHED_TO_ORIGIN" = true || "$PR_CREATED" = true ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES"
fi
echo "================================================"
