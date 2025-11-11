#!/bin/bash

set -euo pipefail

# Glo-Grafana Release Script
# Creates release branch and updates tekton pipeline configurations
#
# Usage:
#   Called by cut-release.sh with environment variables pre-configured
#
# Required environment variables (set by cut-release.sh):
#   RELEASE_BRANCH    - Release branch name (e.g., release-2.17)
#   GH_VERSION        - Global Hub version (e.g., v1.8.0)
#   GRAFANA_BRANCH    - Grafana branch name (e.g., release-1.8)
#   GRAFANA_TAG       - Grafana tag (e.g., globalhub-1-8)

# Configuration
GRAFANA_REPO="stolostron/glo-grafana"
WORK_DIR="${WORK_DIR:-/tmp/globalhub-release-repos}"

# Validate required environment variables
if [[ -z "$RELEASE_BRANCH" || -z "$GH_VERSION" || -z "$GRAFANA_BRANCH" || -z "$GRAFANA_TAG" ]]; then
  echo "❌ Error: Required environment variables not set" >&2
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GRAFANA_BRANCH, GRAFANA_TAG"
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

echo "🚀 Glo-Grafana Release Branch Creation"
echo "$SEPARATOR_LINE"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Grafana Branch: $GRAFANA_BRANCH"
echo "   Grafana Tag: $GRAFANA_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/glo-grafana"
mkdir -p "$WORK_DIR"

# Remove existing directory for clean clone
if [[ -d "$REPO_PATH" ]]; then
  echo "   Removing existing directory for clean clone..."
  rm -rf "$REPO_PATH"
fi

echo "📥 Cloning $GRAFANA_REPO..."
git clone --progress "https://github.com/$GRAFANA_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
if [[ ! -d "$REPO_PATH/.git" ]]; then
  echo "❌ Failed to clone $GRAFANA_REPO" >&2
  exit 1
fi
echo "✅ Cloned successfully"

cd "$REPO_PATH"

# Setup user's fork remote
FORK_REPO="git@github.com:${GITHUB_USER}/glo-grafana.git"
git remote add fork "$FORK_REPO" 2>/dev/null || true

# Check if fork exists
FORK_EXISTS=false
if git ls-remote "$FORK_REPO" HEAD >/dev/null 2>&1; then
  FORK_EXISTS=true
  echo "   ✅ Fork detected: ${GITHUB_USER}/glo-grafana"
else
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/glo-grafana" >&2
fi

# Fetch all release branches
echo "🔄 Fetching release branches..."
git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
echo "   ✅ Release branches fetched"

# Find latest grafana release branch
LATEST_GRAFANA_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

# Check if target branch is the same as latest
if [[ "$LATEST_GRAFANA_RELEASE" = "$GRAFANA_BRANCH" ]]; then
  echo "ℹ️  Target grafana branch is the latest: $GRAFANA_BRANCH"
  echo ""
  echo "   https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
  echo ""
  echo "   Will verify and update if needed..."
  echo ""
fi

if [[ -z "$LATEST_GRAFANA_RELEASE" ]]; then
  echo "⚠️  No previous grafana release branch found, using main as base" >&2
  BASE_BRANCH="main"
else
  echo "Latest grafana release detected: $LATEST_GRAFANA_RELEASE"

  # If target branch is the latest, use second-to-latest as base
  # If target branch is not the latest, use latest as base
  if [[ "$LATEST_GRAFANA_RELEASE" = "$GRAFANA_BRANCH" ]]; then
    # Target is latest - get second-to-latest for base
    SECOND_TO_LATEST=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
      sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
    if [[ -n "$SECOND_TO_LATEST" && "$SECOND_TO_LATEST" != "$GRAFANA_BRANCH" ]]; then
      BASE_BRANCH="$SECOND_TO_LATEST"
      echo "Target is latest release, using previous release as base: $BASE_BRANCH"
    else
      BASE_BRANCH="main"
      echo "No previous release found, using main as base"
    fi
  else
    # Target is not latest - use latest as base
    BASE_BRANCH="$LATEST_GRAFANA_RELEASE"
    echo "Target is older than latest, using latest as base: $BASE_BRANCH"
  fi
fi

# Extract previous Grafana tag info
if [[ "$BASE_BRANCH" != "main" ]]; then
  PREV_GRAFANA_VERSION="${BASE_BRANCH#release-}"
  PREV_GRAFANA_TAG="globalhub-${PREV_GRAFANA_VERSION//./-}"
  echo "Previous Grafana tag: $PREV_GRAFANA_TAG"
else
  PREV_GRAFANA_TAG=""
fi

echo ""

# Initialize tracking variables
BRANCH_EXISTS_ON_ORIGIN=false
CHANGES_COMMITTED=false
PUSHED_TO_ORIGIN=false
GRAFANA_PR_STATUS="none"  # none, created, updated, exists, skipped, pushed
GRAFANA_PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$GRAFANA_BRANCH" | grep -q "$GRAFANA_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $GRAFANA_BRANCH already exists on origin"
  git fetch origin "$GRAFANA_BRANCH" 2>/dev/null || true
  git checkout -B "$GRAFANA_BRANCH" "origin/$GRAFANA_BRANCH"
else
  if [[ "$CREATE_BRANCHES" != "true" ]]; then
    echo "❌ Error: Branch $GRAFANA_BRANCH does not exist on origin" >&2
    echo "   Run with CREATE_BRANCHES=true to create the branch"
    exit 1
  fi
  echo "🌿 Creating $GRAFANA_BRANCH from origin/$BASE_BRANCH..."
  git branch -D "$GRAFANA_BRANCH" 2>/dev/null || true
  git checkout -b "$GRAFANA_BRANCH" "origin/$BASE_BRANCH"
  echo "✅ Created local branch $GRAFANA_BRANCH"
fi

# Step 1: Update tekton pipeline files
echo ""
echo "📍 Step 1: Updating tekton pipeline files..."

if [[ -n "$PREV_GRAFANA_TAG" ]]; then
  # Update pull-request pipeline
  OLD_PR_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-pull-request.yaml"

  if [[ "$PREV_GRAFANA_TAG" = "$GRAFANA_TAG" ]]; then
    # Same tag - just update in place
    if [[ -f "$NEW_PR_PIPELINE" ]]; then
      echo "   ℹ️  Pull-request pipeline already exists with correct name"
      echo "   Updating references in $NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Updated $NEW_PR_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PR_PIPELINE" >&2
    fi
  elif [[ -f "$OLD_PR_PIPELINE" ]]; then
    echo "   Renaming and updating pull-request pipeline..."
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PR_PIPELINE"
    echo "   ✅ Updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE" >&2
  fi

  # Update push pipeline
  OLD_PUSH_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-push.yaml"

  if [[ "$PREV_GRAFANA_TAG" = "$GRAFANA_TAG" ]]; then
    # Same tag - just update in place
    if [[ -f "$NEW_PUSH_PIPELINE" ]]; then
      echo "   ℹ️  Push pipeline already exists with correct name"
      echo "   Updating references in $NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PUSH_PIPELINE"
      echo "   ✅ Updated $NEW_PUSH_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PUSH_PIPELINE" >&2
    fi
  elif [[ -f "$OLD_PUSH_PIPELINE" ]]; then
    echo "   Renaming and updating push pipeline..."
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PUSH_PIPELINE"
    echo "   ✅ Updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE" >&2
  fi
else
  echo "   ⚠️  No previous grafana tag found, skipping pipeline update" >&2
fi

# Step 2: Commit changes
echo ""
echo "📍 Step 2: Committing changes on $GRAFANA_BRANCH..."

if git diff --quiet && git diff --cached --quiet; then
  echo "   ℹ️  No changes to commit"
  CHANGES_COMMITTED=false
else
  git add -A

  COMMIT_MSG="Update glo-grafana for ${GRAFANA_BRANCH} (Global Hub ${GH_VERSION})

- Rename and update pull-request pipeline for ${GRAFANA_TAG}
- Rename and update push pipeline for ${GRAFANA_TAG}
- Update branch references to ${GRAFANA_BRANCH}

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
    echo "   Pushing new branch $GRAFANA_BRANCH to origin..."
    if git push origin "$GRAFANA_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin: $GRAFANA_REPO/$GRAFANA_BRANCH"
      PUSHED_TO_ORIGIN=true
      GRAFANA_PR_STATUS="pushed"
    else
      echo "   ❌ Failed to push branch to origin" >&2
      exit 1
    fi
  else
    # Branch exists or UPDATE mode - create PR to update it
    echo "   Creating PR to update $GRAFANA_BRANCH..."

    # Use fixed branch name for PR deduplication
    PR_BRANCH="${GRAFANA_BRANCH}-update"

    # Check if branch already exists locally and delete it
    if git show-ref --verify --quiet "refs/heads/$PR_BRANCH"; then
      git branch -D "$PR_BRANCH" 2>/dev/null || true
    fi

    git checkout -b "$PR_BRANCH"

    # Check if PR already exists (search by title)
    echo "   Checking for existing PR to $GRAFANA_BRANCH..."
    PR_TITLE="Update ${GRAFANA_BRANCH} grafana configuration"

    EXISTING_PR=$(gh pr list \
      --repo "${GRAFANA_REPO}" \
      --base "$GRAFANA_BRANCH" \
      --state open \
      --search "\"${PR_TITLE}\" in:title author:${GITHUB_USER}" \
      --json number,url,headRefName \
      --jq '.[0] | select(. != null) | "\(.url)|\(.headRefName)"' 2>/dev/null || echo "")

    if [[ -n "$EXISTING_PR" ]]; then
      GRAFANA_PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f1)
      EXISTING_BRANCH=$(echo "$EXISTING_PR" | cut -d'|' -f2)

      echo "   ℹ️  PR already exists: $GRAFANA_PR_URL"
      echo "   Existing branch: ${GITHUB_USER}:${EXISTING_BRANCH}"

      # Check if we can update (same branch name)
      if [[ "$EXISTING_BRANCH" != "$PR_BRANCH" ]]; then
        # Different branch - cannot update
        echo "   ⚠️  Existing PR uses different branch: $EXISTING_BRANCH"
        echo "   Current branch would be: $PR_BRANCH"
        echo "   Please close the existing PR to create a new one, or update manually"
        GRAFANA_PR_STATUS="exists"
      else
        # Same branch - push updates
        echo "   Pushing updates to existing PR branch..."
        if [[ "$FORK_EXISTS" = false ]]; then
          echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
          echo "   Please fork ${GRAFANA_REPO} to enable PR creation"
          GRAFANA_PR_STATUS="exists"
        elif git push -f fork "$PR_BRANCH" 2>&1; then
          echo "   ✅ PR updated with latest changes: $GRAFANA_PR_URL"
          GRAFANA_PR_STATUS="updated"
        else
          echo "   ⚠️  Failed to push updates to fork" >&2
          GRAFANA_PR_STATUS="exists"
        fi
      fi
    else
      # No existing PR, create a new one
      if [[ "$FORK_EXISTS" = false ]]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist" >&2
        echo "   Please fork ${GRAFANA_REPO} to enable PR creation"
        GRAFANA_PR_STATUS="failed"
      else
        echo "   Pushing $PR_BRANCH to fork..."
        if git push -f fork "$PR_BRANCH" 2>&1; then
          echo "   ✅ PR branch pushed to fork"

          PR_BODY="Update ${GRAFANA_BRANCH} grafana configuration

## Changes

- Rename and update pull-request pipeline for \`${GRAFANA_TAG}\`
- Rename and update push pipeline for \`${GRAFANA_TAG}\`
- Update branch references to \`${GRAFANA_BRANCH}\`

## Version Mapping

- **ACM**: ${RELEASE_BRANCH}
- **Global Hub**: ${GH_VERSION}
- **Grafana branch**: ${GRAFANA_BRANCH}
- **Previous release**: ${BASE_BRANCH}"

          PR_CREATE_OUTPUT=$(gh pr create --base "$GRAFANA_BRANCH" --head "${GITHUB_USER}:$PR_BRANCH" \
            --title "Update ${GRAFANA_BRANCH} grafana configuration" \
            --body "$PR_BODY" \
            --repo "$GRAFANA_REPO" 2>&1) || true

          if [[ "$PR_CREATE_OUTPUT" =~ ^https:// ]]; then
            GRAFANA_PR_URL="$PR_CREATE_OUTPUT"
            echo "   ✅ PR created: $GRAFANA_PR_URL"
            GRAFANA_PR_STATUS="created"
          elif [[ "$PR_CREATE_OUTPUT" =~ (https://github.com/[^[:space:]]+) ]]; then
            GRAFANA_PR_URL="${BASH_REMATCH[1]}"
            echo "   ✅ PR created: $GRAFANA_PR_URL"
            GRAFANA_PR_STATUS="created"
          else
            echo "   ⚠️  Failed to create PR" >&2
            echo "   Reason: $PR_CREATE_OUTPUT"
            GRAFANA_PR_STATUS="failed"
          fi
        else
          echo "   ❌ Failed to push PR branch" >&2
          GRAFANA_PR_STATUS="failed"
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
echo "Release: $RELEASE_BRANCH / $GRAFANA_BRANCH"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Grafana branch: $GRAFANA_BRANCH (from $BASE_BRANCH)"
if [[ "$CHANGES_COMMITTED" = true ]]; then
  if [[ -n "$PREV_GRAFANA_TAG" ]]; then
    echo "  ✓ Renamed tekton pipelines (${PREV_GRAFANA_TAG} → ${GRAFANA_TAG})"
  else
    echo "  ✓ Updated tekton pipelines to ${GRAFANA_TAG}"
  fi
fi
case "$GRAFANA_PR_STATUS" in
  "pushed")
    echo "  ✓ Pushed to origin: ${GRAFANA_REPO}/${GRAFANA_BRANCH}"
    ;;
  "created")
    echo "  ✓ PR to $GRAFANA_BRANCH: Created - ${GRAFANA_PR_URL}"
    ;;
  "updated")
    echo "  ✓ PR to $GRAFANA_BRANCH: Updated - ${GRAFANA_PR_URL}"
    ;;
  "exists")
    echo "  ✓ PR to $GRAFANA_BRANCH: Exists (no changes) - ${GRAFANA_PR_URL}"
    ;;
esac
echo ""
echo "$SEPARATOR_LINE"
echo "📝 NEXT STEPS"
echo "$SEPARATOR_LINE"
if [[ "$GRAFANA_PR_STATUS" = "pushed" ]]; then
  echo "✅ Branch pushed to origin successfully"
  echo ""
  echo "Branch: https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
  echo ""
  echo "Verify: Tekton pipelines and grafana images"
elif [[ "$GRAFANA_PR_STATUS" = "created" || "$GRAFANA_PR_STATUS" = "updated" || "$GRAFANA_PR_STATUS" = "exists" ]]; then
  case "$GRAFANA_PR_STATUS" in
    "created")
      echo "1. Review and merge PR to $GRAFANA_BRANCH:"
      ;;
    "updated")
      echo "1. Review updated PR to $GRAFANA_BRANCH:"
      ;;
    "exists")
      echo "1. Review existing PR to $GRAFANA_BRANCH:"
      ;;
  esac
  echo "   ${GRAFANA_PR_URL}"
  echo ""
  echo "After merge: Verify tekton pipelines and grafana images"
else
  echo "Repository: https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
fi
echo ""
echo "$SEPARATOR_LINE"
if [[ "$GRAFANA_PR_STATUS" = "pushed" || "$GRAFANA_PR_STATUS" = "created" || "$GRAFANA_PR_STATUS" = "updated" ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES" >&2
fi
echo "$SEPARATOR_LINE"
