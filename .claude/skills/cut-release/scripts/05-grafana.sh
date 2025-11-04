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
  echo "❌ Error: Required environment variables not set"
  echo "   This script should be called by cut-release.sh"
  echo "   Required: RELEASE_BRANCH, GH_VERSION, GRAFANA_BRANCH, GRAFANA_TAG"
  exit 1
fi

# Detect OS and set sed in-place flag
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED_INPLACE=(-i "")
else
  SED_INPLACE=(-i)
fi

echo "🚀 Glo-Grafana Release Branch Creation"
echo "================================================"
echo "   ACM Release Branch: $RELEASE_BRANCH"
echo "   Global Hub Version: $GH_VERSION"
echo "   Grafana Branch: $GRAFANA_BRANCH"
echo "   Grafana Tag: $GRAFANA_TAG"
echo ""

# Setup repository
REPO_PATH="$WORK_DIR/glo-grafana"
mkdir -p "$WORK_DIR"

# Remove existing directory for clean clone
if [ -d "$REPO_PATH" ]; then
  echo "   Removing existing directory for clean clone..."
  rm -rf "$REPO_PATH"
fi

echo "📥 Cloning $GRAFANA_REPO..."
git clone --progress "https://github.com/$GRAFANA_REPO.git" "$REPO_PATH" 2>&1 | grep -E "Receiving|Resolving|Cloning" || true
if [ ! -d "$REPO_PATH/.git" ]; then
  echo "❌ Failed to clone $GRAFANA_REPO"
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
  echo "   ⚠️  Fork not found: ${GITHUB_USER}/glo-grafana"
fi

# Fetch all release branches
echo "🔄 Fetching release branches..."
git fetch origin 'refs/heads/release-*:refs/remotes/origin/release-*' --progress 2>&1 | grep -E "Receiving|Resolving|new branch" || true
echo "   ✅ Release branches fetched"

# Find latest grafana release branch
LATEST_GRAFANA_RELEASE=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
  sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -1)

# Check if target branch is the same as latest
if [ "$LATEST_GRAFANA_RELEASE" = "$GRAFANA_BRANCH" ]; then
  echo "ℹ️  Target grafana branch is the latest: $GRAFANA_BRANCH"
  echo ""
  echo "   https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
  echo ""
  echo "   Will verify and update if needed..."
  echo ""
fi

if [ -z "$LATEST_GRAFANA_RELEASE" ]; then
  echo "⚠️  No previous grafana release branch found, using main as base"
  BASE_BRANCH="main"
else
  echo "Latest grafana release detected: $LATEST_GRAFANA_RELEASE"

  # If target branch is the latest, use second-to-latest as base
  # If target branch is not the latest, use latest as base
  if [ "$LATEST_GRAFANA_RELEASE" = "$GRAFANA_BRANCH" ]; then
    # Target is latest - get second-to-latest for base
    SECOND_TO_LATEST=$(git branch -r | grep -E 'origin/release-[0-9]+\.[0-9]+$' | \
      sed 's|.*origin/||' | sed 's|^[* ]*||' | sort -V | tail -2 | head -1)
    if [ -n "$SECOND_TO_LATEST" ] && [ "$SECOND_TO_LATEST" != "$GRAFANA_BRANCH" ]; then
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
if [ "$BASE_BRANCH" != "main" ]; then
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
PR_CREATED=false
PR_URL=""

# Check if new release branch already exists on origin (upstream)
if git ls-remote --heads origin "$GRAFANA_BRANCH" | grep -q "$GRAFANA_BRANCH"; then
  BRANCH_EXISTS_ON_ORIGIN=true
  echo "ℹ️  Branch $GRAFANA_BRANCH already exists on origin"
  git fetch origin "$GRAFANA_BRANCH" 2>/dev/null || true
  git checkout -B "$GRAFANA_BRANCH" "origin/$GRAFANA_BRANCH"
else
  if [ "$CUT_MODE" != "true" ]; then
    echo "❌ Error: Branch $GRAFANA_BRANCH does not exist on origin"
    echo "   Run with CUT_MODE=true to create the branch"
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

if [ -n "$PREV_GRAFANA_TAG" ]; then
  # Update pull-request pipeline
  OLD_PR_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-pull-request.yaml"
  NEW_PR_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-pull-request.yaml"

  if [ "$PREV_GRAFANA_TAG" = "$GRAFANA_TAG" ]; then
    # Same tag - just update in place
    if [ -f "$NEW_PR_PIPELINE" ]; then
      echo "   ℹ️  Pull-request pipeline already exists with correct name"
      echo "   Updating references in $NEW_PR_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PR_PIPELINE"
      echo "   ✅ Updated $NEW_PR_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PR_PIPELINE"
    fi
  elif [ -f "$OLD_PR_PIPELINE" ]; then
    echo "   Renaming and updating pull-request pipeline..."
    git mv "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE" 2>/dev/null || cp "$OLD_PR_PIPELINE" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PR_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PR_PIPELINE"
    echo "   ✅ Updated $NEW_PR_PIPELINE"
  else
    echo "   ⚠️  Previous pull-request pipeline not found: $OLD_PR_PIPELINE"
  fi

  # Update push pipeline
  OLD_PUSH_PIPELINE=".tekton/glo-grafana-${PREV_GRAFANA_TAG}-push.yaml"
  NEW_PUSH_PIPELINE=".tekton/glo-grafana-${GRAFANA_TAG}-push.yaml"

  if [ "$PREV_GRAFANA_TAG" = "$GRAFANA_TAG" ]; then
    # Same tag - just update in place
    if [ -f "$NEW_PUSH_PIPELINE" ]; then
      echo "   ℹ️  Push pipeline already exists with correct name"
      echo "   Updating references in $NEW_PUSH_PIPELINE"
      sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PUSH_PIPELINE"
      echo "   ✅ Updated $NEW_PUSH_PIPELINE"
    else
      echo "   ⚠️  Pipeline not found: $NEW_PUSH_PIPELINE"
    fi
  elif [ -f "$OLD_PUSH_PIPELINE" ]; then
    echo "   Renaming and updating push pipeline..."
    git mv "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE" 2>/dev/null || cp "$OLD_PUSH_PIPELINE" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${PREV_GRAFANA_TAG}/${GRAFANA_TAG}/g" "$NEW_PUSH_PIPELINE"
    sed "${SED_INPLACE[@]}" "s/${BASE_BRANCH}/${GRAFANA_BRANCH}/g" "$NEW_PUSH_PIPELINE"
    echo "   ✅ Updated $NEW_PUSH_PIPELINE"
  else
    echo "   ⚠️  Previous push pipeline not found: $OLD_PUSH_PIPELINE"
  fi
else
  echo "   ⚠️  No previous grafana tag found, skipping pipeline update"
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

if [ "$CHANGES_COMMITTED" = false ]; then
  echo "   ℹ️  No changes to publish"
else
  # Decision: Push directly or create PR based on CUT_MODE and branch existence
  if [ "$CUT_MODE" = "true" ] && [ "$BRANCH_EXISTS_ON_ORIGIN" = false ]; then
    # CUT mode + branch doesn't exist - push directly
    echo "   Pushing new branch $GRAFANA_BRANCH to origin..."
    if git push origin "$GRAFANA_BRANCH" 2>&1; then
      echo "   ✅ Branch pushed to origin: $GRAFANA_REPO/$GRAFANA_BRANCH"
      PUSHED_TO_ORIGIN=true
    else
      echo "   ❌ Failed to push branch to origin"
      exit 1
    fi
  else
    # Branch exists or UPDATE mode - create PR to update it
    echo "   Creating PR to update $GRAFANA_BRANCH..."

    # Check if PR already exists
    echo "   Checking for existing PR to $GRAFANA_BRANCH..."
    EXISTING_PR=$(gh pr list \
      --repo "${GRAFANA_REPO}" \
      --base "$GRAFANA_BRANCH" \
      --state open \
      --search "Update ${GRAFANA_BRANCH} grafana configuration" \
      --json number,url,state \
      --jq '.[0] | select(. != null) | "\(.state)|\(.url)"' 2>/dev/null || echo "")

    if [ -n "$EXISTING_PR" ] && [ "$EXISTING_PR" != "null|null" ]; then
      PR_STATE=$(echo "$EXISTING_PR" | cut -d'|' -f1)
      PR_URL=$(echo "$EXISTING_PR" | cut -d'|' -f2)
      echo "   ℹ️  PR already exists (state: $PR_STATE): $PR_URL"
      PR_CREATED=true
    else
      PR_BRANCH="${GRAFANA_BRANCH}-update-$(date +%s)"
      git checkout -b "$PR_BRANCH"

      if [ "$FORK_EXISTS" = false ]; then
        echo "   ⚠️  Cannot push to fork - fork does not exist"
        echo "   Please fork ${GRAFANA_REPO} to enable PR creation"
        PR_CREATED=false
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
          echo "   ❌ Failed to push PR branch"
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
echo "Release: $RELEASE_BRANCH / $GRAFANA_BRANCH"
echo ""
echo "✅ COMPLETED TASKS:"
echo "  ✓ Grafana branch: $GRAFANA_BRANCH (from $BASE_BRANCH)"
if [ "$CHANGES_COMMITTED" = true ]; then
  if [ -n "$PREV_GRAFANA_TAG" ]; then
    echo "  ✓ Renamed tekton pipelines (${PREV_GRAFANA_TAG} → ${GRAFANA_TAG})"
  else
    echo "  ✓ Updated tekton pipelines to ${GRAFANA_TAG}"
  fi
fi
if [ "$PUSHED_TO_ORIGIN" = true ]; then
  echo "  ✓ Pushed to origin: ${GRAFANA_REPO}/${GRAFANA_BRANCH}"
fi
if [ "$PR_CREATED" = true ] && [ -n "$PR_URL" ]; then
  echo "  ✓ PR to $GRAFANA_BRANCH: ${PR_URL}"
fi
echo ""
echo "================================================"
echo "📝 NEXT STEPS"
echo "================================================"
if [ "$PUSHED_TO_ORIGIN" = true ]; then
  echo "✅ Branch pushed to origin successfully"
  echo ""
  echo "Branch: https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
  echo ""
  echo "Verify: Tekton pipelines and grafana images"
elif [ "$PR_CREATED" = true ]; then
  echo "1. Review and merge PR to $GRAFANA_BRANCH:"
  echo "   ${PR_URL}"
  echo ""
  echo "After merge: Verify tekton pipelines and grafana images"
else
  echo "Repository: https://github.com/$GRAFANA_REPO/tree/$GRAFANA_BRANCH"
fi
echo ""
echo "================================================"
if [[ "$PUSHED_TO_ORIGIN" = true || "$PR_CREATED" = true ]]; then
  echo "✅ SUCCESS"
else
  echo "⚠️  COMPLETED WITH ISSUES"
fi
echo "================================================"
