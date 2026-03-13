#!/bin/bash

set -euo pipefail

# Postgres Exporter Release Script
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
#   POSTGRES_TAG      - Postgres tag (e.g., globalhub-1-8)
#   GITHUB_USER       - GitHub username for PR creation
#   CREATE_BRANCHES          - true: create branch, false: update via PR

# Configuration
POSTGRES_REPO="stolostron/postgres_exporter"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$POSTGRES_TAG" || -z "$GITHUB_USER" || -z "$CREATE_BRANCHES" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, POSTGRES_TAG, GITHUB_USER, CREATE_BRANCHES"
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
readonly PR_STATUS_PUSHED='pushed'
readonly PR_STATUS_CREATED='created'
readonly PR_STATUS_UPDATED='updated'
readonly PR_STATUS_EXISTS='exists'

echo "🚀 Postgres Exporter Release"
echo "$SEPARATOR_LINE"
echo "   Mode: $([[ "$CREATE_BRANCHES" = true ]] && echo "CUT (create branch)" || echo "UPDATE (PR only)")"
echo "   Release: $RELEASE_BRANCH"
echo "   Postgres Tag: $POSTGRES_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/postgres_exporter"
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
  git fetch origin --progress 2>&1 | grep -E "Receiving|Resolving|Fetching" || true
  echo "   ✅ Repository updated"
else
  echo "📥 Cloning $POSTGRES_REPO..."
  git clone --progress "https://github.com/$POSTGRES_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
  if [[ ! -d "$REPO_PATH/.git" ]]; then
    echo "❌ Failed to clone $POSTGRES_REPO" >&2
    exit 1
  fi
  echo "✅ Cloned successfully"
  cd "$REPO_PATH"
fi

# Setup user's fork remote (if not already added)
FORK_REPO="git@github.com:${GITHUB_USER}/postgres_exporter.git"
if ! git remote | grep -q "^fork$"; then
  git remote add fork "$FORK_REPO" 2>/dev/null || true
fi

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/postgres_exporter"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/postgres_exporter" >&2
fi

# Calculate previous postgres tag directly from version formula
# RELEASE_BRANCH e.g. "release-2.17" -> prev ACM minor = 16 -> base = release-2.16
# GH_VERSION_SHORT e.g. "1.8" -> prev GH minor = 7 -> globalhub-1-7
ACM_MINOR="${RELEASE_BRANCH##*.}"
PREV_ACM_MINOR=$((ACM_MINOR - 1))
BASE_BRANCH="release-2.${PREV_ACM_MINOR}"
GH_MINOR="${GH_VERSION_SHORT#*.}"
PREV_GH_MINOR=$((GH_MINOR - 1))
PREV_POSTGRES_TAG="globalhub-1-${PREV_GH_MINOR}"
echo "Previous Postgres tag: $PREV_POSTGRES_TAG (base: $BASE_BRANCH)"

echo ""

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
POSTGRES_PR_STATUS="none"  # none, created, updated, exists, skipped, pushed
POSTGRES_PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$RELEASE_BRANCH" | grep -q "$RELEASE_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $RELEASE_BRANCH already exists on origin"
  git fetch origin "$RELEASE_BRANCH" 2>/dev/null || true
  git checkout -B "$RELEASE_BRANCH" "origin/$RELEASE_BRANCH"
else
  if [[ "$CREATE_BRANCHES" != "true" ]]; then
    echo "❌ Error: Branch $RELEASE_BRANCH does not exist on origin" >&2
    echo "   Run with CREATE_BRANCHES=true to create the branch"
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
      echo "   ⚠️  Pipeline not found: $NEW_PR_PIPELINE" >&2
    fi
  elif [[ -f "$OLD_PR_PIPELINE" ]]; then
    echo "   Renaming and updating pull-request pipeline..."
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PR_PIPELINE"
    echo "   ✅ Updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE" >&2
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
      echo "   ⚠️  Pipeline not found: $NEW_PUSH_PIPELINE" >&2
    fi
  elif [[ -f "$OLD_PUSH_PIPELINE" ]]; then
    echo "   Renaming and updating push pipeline..."
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_POSTGRES_TAG}/${POSTGRES_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${RELEASE_BRANCH}/g" "$NEW_PUSH_PIPELINE"
    echo "   ✅ Updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE" >&2
  fi
else
  echo "   ⚠️  No previous postgres tag found, skipping pipeline update" >&2
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
  # Decision: Push directly or create PR based on CREATE_BRANCHES and branch existence
  if [[ "$CREATE_BRANCHES" = "true" && "$BRANCH_EXISTS_ON_ORIGIN" = false ]]; then
    # CUT mode + branch doesn't exist - push directly
    echo "   Pushing new branch $RELEASE_BRANCH to origin..."
    if git push origin "$RELEASE_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin: $POSTGRES_REPO/$RELEASE_BRANCH"
      PUSHED_TO_ORIGIN=true
      POSTGRES_PR_STATUS="$PR_STATUS_PUSHED"
    else
      echo "   ❌ Failed to push branch to origin" >&2
      exit 1
    fi
  else
    # Branch exists or UPDATE mode - create PR to update it
    echo "   Creating PR to update $RELEASE_BRANCH..."

    # Use fixed branch name for PR deduplication
    PR_BRANCH="${RELEASE_BRANCH}-update"

    # Check if branch already exists locally and delete it
    if git show-ref --verify --quiet "refs/heads/$PR_BRANCH"; then
      git branch -D "$PR_BRANCH" 2>/dev/null || true
    fi

    git checkout -b "$PR_BRANCH"

    # Check if PR already exists (search by title)
    echo "   Checking for existing PR to $RELEASE_BRANCH..."
    PR_TITLE="Update ${RELEASE_BRANCH} postgres_exporter configuration"

    EXISTING_PR=$(gh pr list \
      --repo "${POSTGRES_REPO}" \
      --base "$RELEASE_BRANCH" \
      --state open \
      --search "\"${PR_TITLE}\" in:title author:${GITHUB_USER}" \
      --json number,url,headRefName \
      --jq '.[0] | select(. != null) | "\(.url)|\(.headRefName)"' 2>/dev/null || echo "")

    if [[ -n "$EXISTING_PR" ]]; then
      POSTGRES_PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f1)
      EXISTING_BRANCH=$(echo "$EXISTING_PR" | cut -d'|' -f2)

      echo "   ℹ️  PR already exists: $POSTGRES_PR_URL"
      echo "   Existing branch: ${GITHUB_USER}:${EXISTING_BRANCH}"

      # Always push updates to the existing PR's branch (even if branch name is different)
      echo "   Pushing updates to existing PR branch: $EXISTING_BRANCH..."
      if [[ "$FORK_EXISTS" = false ]]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
        echo "   Please fork ${POSTGRES_REPO} to enable PR creation"
        POSTGRES_PR_STATUS="exists"
      elif git push -f fork "$RELEASE_BRANCH:$EXISTING_BRANCH" 2>&1; then
        echo "   ✅ PR updated with latest changes: $POSTGRES_PR_URL"
        POSTGRES_PR_STATUS="updated"
      else
        echo "   ⚠️  Failed to push updates to fork" >&2
        POSTGRES_PR_STATUS="exists"
      fi
    else
      # No existing PR, create a new one
      if [[ "$FORK_EXISTS" = false ]]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
        echo "   Please fork ${POSTGRES_REPO} to enable PR creation"
        POSTGRES_PR_STATUS="failed"
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
            POSTGRES_PR_URL="$PR_CREATE_OUTPUT"
            echo "   ✅ PR created: $POSTGRES_PR_URL"
            POSTGRES_PR_STATUS="created"
          elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
            POSTGRES_PR_URL="${BASH_REMATCH[1]}"
            echo "   ✅ PR created: $POSTGRES_PR_URL"
            POSTGRES_PR_STATUS="created"
          else
            echo "   ⚠️  Failed to create PR" >&2
            echo "   Reason: $PR_CREATE_OUTPUT"
            POSTGRES_PR_STATUS="failed"
          fi
        else
          echo "   ❌ Failed to push PR branch" >&2
          POSTGRES_PR_STATUS="failed"
        fi
      fi
    fi
  fi
fi

# Summary
echo ""
echo "$SEPARATOR_LINE"
echo "📊 WORKFLOW SUMMARY"
echo "$SEPARATOR_LINE"
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
case "$POSTGRES_PR_STATUS" in
  "$PR_STATUS_PUSHED")
    echo "  ✓ Pushed to origin: ${POSTGRES_REPO}/${RELEASE_BRANCH}"
    ;;
  "$PR_STATUS_CREATED")
    echo "  ✓ PR to $RELEASE_BRANCH: Created - ${POSTGRES_PR_URL}"
    ;;
  "$PR_STATUS_UPDATED")
    echo "  ✓ PR to $RELEASE_BRANCH: Updated - ${POSTGRES_PR_URL}"
    ;;
  "$PR_STATUS_EXISTS")
    echo "  ✓ PR to $RELEASE_BRANCH: Exists (no changes) - ${POSTGRES_PR_URL}"
    ;;
  *)
    echo "  ⚠️  Unknown PR status: $POSTGRES_PR_STATUS" >&2
    ;;
esac
echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"
if [[ "$POSTGRES_PR_STATUS" = "$PR_STATUS_PUSHED" ]]; then
  echo "✅ Branch pushed to origin successfully"
  echo ""
  echo "Branch: https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
  echo ""
  echo "Verify: Tekton pipelines and postgres exporter images"
elif [[ "$POSTGRES_PR_STATUS" = "$PR_STATUS_CREATED" || "$POSTGRES_PR_STATUS" = "$PR_STATUS_UPDATED" || "$POSTGRES_PR_STATUS" = "$PR_STATUS_EXISTS" ]]; then
  case "$POSTGRES_PR_STATUS" in
    "$PR_STATUS_CREATED")
      echo "1. Review and merge PR to $RELEASE_BRANCH:"
      ;;
    "$PR_STATUS_UPDATED")
      echo "1. Review updated PR to $RELEASE_BRANCH:"
      ;;
    "$PR_STATUS_EXISTS")
      echo "1. Review existing PR to $RELEASE_BRANCH:"
      ;;
    *)
      echo "1. Review PR to $RELEASE_BRANCH (status: $POSTGRES_PR_STATUS):"
      ;;
  esac
  echo "   ${POSTGRES_PR_URL}"
  echo ""
  echo "After merge: Verify tekton pipelines and postgres exporter images"
else
  echo "Repository: https://github.com/$POSTGRES_REPO/tree/$RELEASE_BRANCH"
fi
echo ""
echo "$SEPARATOR_LINE"
if [[ "$POSTGRES_PR_STATUS" = "$PR_STATUS_PUSHED" || "$POSTGRES_PR_STATUS" = "$PR_STATUS_CREATED" || "$POSTGRES_PR_STATUS" = "$PR_STATUS_UPDATED" ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES" >&2
fi
echo "$SEPARATOR_LINE"
