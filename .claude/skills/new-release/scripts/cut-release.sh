#!/bin/bash

set -euo pipefail

# Global Hub Complete Release Workflow
# Orchestrates release process across all repositories
#
# Usage:
#   RELEASE_BRANCH=release-2.17 ./cut-release.sh             # Interactive mode - choose which repos to update
#   RELEASE_BRANCH=release-2.17 ./cut-release.sh all         # Update all repositories
#   RELEASE_BRANCH=release-2.17 ./cut-release.sh 1           # Update only multicluster-global-hub
#   RELEASE_BRANCH=release-2.17 ./cut-release.sh 1,2,3       # Update specific repositories (comma-separated)
#
#   CREATE_BRANCHES=true RELEASE_BRANCH=release-2.17 ./cut-release.sh all  # CREATE_BRANCHES mode - create and push release branches directly to upstream
#
# Note: RELEASE_BRANCH environment variable is REQUIRED (e.g., release-2.14, release-2.15, release-2.16, release-2.17)
#
# Dependencies:
#   - Script 3 (operator-bundle) requires script 1 (multicluster-global-hub) to run first
#   - Script 4 (operator-catalog) requires script 1 (multicluster-global-hub) to run first
#   The script will automatically check and enforce these dependencies

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Repository information (Bash 3.2 compatible - using indexed arrays)
REPO_1="multicluster-global-hub|01-multicluster-global-hub.sh|Main repository with operator, manager, and agent"
REPO_2="openshift/release|02-openshift-release.sh|OpenShift CI configuration"
REPO_3="operator-bundle|03-bundle.sh|Operator bundle manifests"
REPO_4="operator-catalog|04-catalog.sh|Operator catalog for OCP versions"
REPO_5="glo-grafana|05-grafana.sh|Grafana dashboards"
REPO_6="postgres_exporter|06-postgres-exporter.sh|Postgres exporter"

# Helper function to get repo info by number
get_repo_info() {
  local num=$1
  local repo_info
  case $num in
    1) repo_info="$REPO_1" ;;
    2) repo_info="$REPO_2" ;;
    3) repo_info="$REPO_3" ;;
    4) repo_info="$REPO_4" ;;
    5) repo_info="$REPO_5" ;;
    6) repo_info="$REPO_6" ;;
    *) repo_info="" ;;
  esac
  echo "$repo_info"
  return 0
}

# Helper function to get dependencies for a script
# Returns space-separated list of required script numbers
get_dependencies() {
  local num=$1
  case $num in
    3) echo "1" ;;  # Script 3 (bundle) requires script 1 (multicluster-global-hub)
    4) echo "1" ;;  # Script 4 (catalog) requires script 1 (multicluster-global-hub)
    *) echo "" ;;
  esac
}

# Parse command line argument
MODE="${1:-interactive}"

# Configuration
RELEASE_BRANCH="${RELEASE_BRANCH:-}"
OPENSHIFT_RELEASE_PATH="${OPENSHIFT_RELEASE_PATH:-/tmp/openshift-release}"
CREATE_BRANCHES="${CREATE_BRANCHES:-false}"

# Constants for repeated patterns
readonly SEPARATOR_LINE='================================================'

# Auto-detect GitHub user from current git repo (can be overridden with GITHUB_USER env var)
if [[ -z "${GITHUB_USER:-}" ]]; then
  GITHUB_USER=$(git remote get-url origin 2>/dev/null | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|' || echo "")
  if [[ -z "$GITHUB_USER" ]]; then
    echo "❌ Error: Could not auto-detect GitHub user from git remote" >&2
    echo "   Please set GITHUB_USER environment variable or run from a git repository"
    exit 1
  fi
fi

export GITHUB_USER
export CREATE_BRANCHES

# RELEASE_BRANCH must be explicitly specified
if [[ -z "$RELEASE_BRANCH" ]]; then
  echo "❌ Error: RELEASE_BRANCH environment variable is required" >&2
  echo ""
  echo "Usage:"
  echo "   RELEASE_BRANCH=release-2.17 $0 [options]"
  echo ""
  echo "Examples:"
  echo "   RELEASE_BRANCH=release-2.17 $0 all          # Update all repositories"
  echo "   RELEASE_BRANCH=release-2.17 $0 1,2,3        # Update specific repositories"
  echo "   RELEASE_BRANCH=release-2.17 $0              # Interactive mode"
  echo ""
  echo "Available release branches: release-2.14, release-2.15, release-2.16, release-2.17, ..."
  exit 1
fi

echo "Using specified release: $RELEASE_BRANCH"

# Calculate all version variables
ACM_VERSION="${RELEASE_BRANCH#release-}"
ACM_MINOR=$(echo "$ACM_VERSION" | cut -d. -f2)

# Global Hub version calculation: v1.X.0 where X = ACM_MINOR - 9
GH_MINOR=$((ACM_MINOR - 9))
GH_VERSION="v1.${GH_MINOR}.0"
GH_VERSION_SHORT="1.${GH_MINOR}"

# Bundle and Catalog use Global Hub version format
BUNDLE_BRANCH="release-${GH_VERSION_SHORT}"
BUNDLE_TAG="globalhub-${GH_VERSION_SHORT//./-}"
CATALOG_BRANCH="release-${GH_VERSION_SHORT}"
CATALOG_TAG="globalhub-${GH_VERSION_SHORT//./-}"

# Grafana and Postgres use Global Hub version format
GRAFANA_BRANCH="release-${GH_VERSION_SHORT}"
GRAFANA_TAG="globalhub-${GH_VERSION_SHORT//./-}"
POSTGRES_TAG="globalhub-${GH_VERSION_SHORT//./-}"

# OCP version calculation for catalog
# Formula: OCP_MIN = 4.(10 + GH_MINOR), OCP_MAX = OCP_MIN + 4
# Example: GH 1.6 → OCP 4.16-4.20, GH 1.7 → OCP 4.17-4.21
OCP_BASE=10
OCP_MIN=$((GH_MINOR + OCP_BASE))
OCP_MAX=$((OCP_MIN + 4))

# Display version information
echo ""
echo "📊 Version Information"
echo "$SEPARATOR_LINE"
echo "   Mode:        $([[ "$CREATE_BRANCHES" = true ]] && echo "CREATE_BRANCHES (create branches)" || echo "UPDATE (PR only)")"
echo "   GitHub User: $GITHUB_USER"
echo "   ACM:         $RELEASE_BRANCH"
echo "   Global Hub:  release-$GH_VERSION_SHORT"
echo "   Bundle:      release-$GH_VERSION_SHORT"
echo "   Catalog:     release-$GH_VERSION_SHORT"
echo "   OCP:         4.${OCP_MIN} - 4.${OCP_MAX}"
echo "$SEPARATOR_LINE"
echo ""

# Export version variables for child scripts
export RELEASE_BRANCH
export ACM_VERSION
export GH_VERSION
export GH_VERSION_SHORT
export BUNDLE_BRANCH
export BUNDLE_TAG
export CATALOG_BRANCH
export CATALOG_TAG
export GRAFANA_BRANCH
export GRAFANA_TAG
export POSTGRES_TAG
export OPENSHIFT_RELEASE_PATH
export OCP_MIN
export OCP_MAX

# Function to display repo list
show_repos() {
  echo "Available repositories:"
  echo ""
  for i in 1 2 3 4 5 6; do
    local repo_info
    local name
    local desc
    local deps
    repo_info=$(get_repo_info "$i")
    IFS='|' read -r name _ desc <<< "$repo_info"
    deps=$(get_dependencies "$i")
    if [[ -n "$deps" ]]; then
      printf "   [%d] %-25s %s (requires: %s)\n" "$i" "$name" "$desc" "$deps"
    else
      printf "   [%d] %-25s %s\n" "$i" "$name" "$desc"
    fi
  done
  echo ""
  echo "Note: Scripts 3 and 4 require script 1 to run first"
  echo ""
  return 0
}

# Function to run a specific script
run_script() {
  local repo_num=$1
  local repo_info
  local name
  local script
  local desc
  repo_info=$(get_repo_info "$repo_num")
  IFS='|' read -r name script desc <<< "$repo_info"

  echo ""
  echo "────────────────────────────────────────────────"
  echo "📦 [$repo_num/6] $name"
  echo "────────────────────────────────────────────────"
  echo "   $desc"
  echo ""

  if [[ ! -f "$SCRIPT_DIR/$script" ]]; then
    echo "❌ Error: Script not found: $script" >&2
    return 1
  fi

  # Make script executable
  chmod +x "$SCRIPT_DIR/$script"

  # Run the script (it will use the exported environment variables)
  if bash "$SCRIPT_DIR/$script"; then
    echo ""
    echo "✅ $name completed successfully"
    return 0
  else
    echo ""
    echo "❌ $name failed" >&2
    return 1
  fi
}

# Determine which repos to update
REPOS_TO_UPDATE=()

case "$MODE" in
  interactive)
    show_repos
    echo "Select repositories to update:"
    echo "   Enter numbers separated by commas (e.g., 1,2,3)"
    echo "   Or press Enter to update all repositories"
    echo ""
    read -r -p "Selection: " selection

    if [[ -z "$selection" ]]; then
      # Update all
      REPOS_TO_UPDATE=(1 2 3 4 5 6)
      echo "Updating all repositories..."
    else
      # Parse comma-separated list
      IFS=',' read -ra REPOS_TO_UPDATE <<< "$selection"
      echo "Updating selected repositories: ${REPOS_TO_UPDATE[*]}"
    fi
    ;;

  all)
    REPOS_TO_UPDATE=(1 2 3 4 5 6)
    echo "Mode: Update all repositories"
    ;;

  *)
    # Parse comma-separated list from argument
    IFS=',' read -ra REPOS_TO_UPDATE <<< "$MODE"
    echo "Mode: Update selected repositories: ${REPOS_TO_UPDATE[*]}"
    ;;
esac

echo ""
echo "🚀 Starting Release Workflow"
echo "$SEPARATOR_LINE"
echo ""

# Track results
TOTAL=${#REPOS_TO_UPDATE[@]}
COMPLETED=0
FAILED=0
FAILED_REPOS=()
EXECUTED_SCRIPTS=()
# Track PR information for each repo
declare -A REPO_STATUS
declare -A REPO_PRS

# Execute selected scripts
for repo_num in "${REPOS_TO_UPDATE[@]}"; do
  # Strip leading zeros (convert 02 to 2, 03 to 3, etc.)
  repo_num=$((10#$repo_num))

  # Validate repo number
  if [[ "$repo_num" -lt 1 || "$repo_num" -gt 6 ]]; then
    echo "⚠️  Invalid repository number: $repo_num (skipping)" >&2
    continue
  fi

  # Check dependencies
  dependencies=$(get_dependencies "$repo_num")
  if [[ -n "$dependencies" ]]; then
    missing_deps=()
    for dep in $dependencies; do
      # Check if dependency was executed in current run or exists from previous run
      dep_in_current_run=false
      for executed in "${EXECUTED_SCRIPTS[@]}"; do
        if [[ "$executed" -eq "$dep" ]]; then
          dep_in_current_run=true
          break
        fi
      done

      if [[ "$dep_in_current_run" = false ]]; then
        # Check if dependency output exists (from previous run)
        dep_satisfied=false
        case $dep in
          1)
            # Script 1 creates multicluster-global-hub-release directory
            if [[ -d "${WORK_DIR:-/tmp/globalhub-release-repos}/multicluster-global-hub-release/.git" ]]; then
              dep_satisfied=true
            fi
            ;;
        esac

        if [[ "$dep_satisfied" = false ]]; then
          missing_deps+=("$dep")
        fi
      fi
    done

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
      repo_info=$(get_repo_info "$repo_num")
      IFS='|' read -r name _ _ <<< "$repo_info"
      echo ""
      echo "❌ Error: Script $repo_num ($name) has unmet dependencies" >&2
      echo "   Missing required scripts: ${missing_deps[*]}" >&2
      echo "   Please run script ${missing_deps[*]} first, or include them in this run" >&2
      echo ""
      FAILED=$((FAILED + 1))
      FAILED_REPOS+=("$name (dependency check failed)")

      # Ask if user wants to continue
      if [[ "$FAILED" -lt "$TOTAL" ]]; then
        echo "⚠️  Continue with remaining repositories? (y/n)" >&2
        read -r continue_choice
        if [[ ! "$continue_choice" =~ ^[Yy]$ ]]; then
          echo "Workflow aborted by user"
          break
        fi
      fi
      continue
    fi
  fi

  repo_info=$(get_repo_info "$repo_num")
  IFS='|' read -r name _ _ <<< "$repo_info"

  # Capture script output to extract PR URLs
  SCRIPT_OUTPUT=$(mktemp)
  if run_script "$repo_num" 2>&1 | tee "$SCRIPT_OUTPUT"; then
    COMPLETED=$((COMPLETED + 1))
    EXECUTED_SCRIPTS+=("$repo_num")
    REPO_STATUS["$repo_num"]="✅ Completed"

    # Extract PR URL from WORKFLOW SUMMARY or NEXT STEPS section (most reliable)
    # First, try to extract from the summary sections at the end of script output
    PR_URL=""

    # Look for PR URLs in NEXT STEPS section (after "NEXT STEPS" line)
    if [[ -z "$PR_URL" ]]; then
      PR_URL=$(awk '/NEXT STEPS/,0' "$SCRIPT_OUTPUT" | grep -E "^(1\.|   ).*Review and merge" | grep -oE 'https://github.com/[^/]+/[^/]+/pull/[0-9]+' | head -1 || echo "")
    fi

    # Look for PR in COMPLETED TASKS section
    if [[ -z "$PR_URL" ]]; then
      PR_URL=$(awk '/COMPLETED TASKS/,/NEXT STEPS/' "$SCRIPT_OUTPUT" | grep -E "✓.*PR.*:" | grep -oE 'https://github.com/[^/]+/[^/]+/pull/[0-9]+' | head -1 || echo "")
    fi

    # Fallback: Look for PR creation/update messages (but only the last one in output to avoid wrong repo)
    if [[ -z "$PR_URL" ]]; then
      PR_URL=$(grep -E "(✅|✓).*(PR created|PR updated|PR already exists and updated):" "$SCRIPT_OUTPUT" | grep -oE 'https://github.com/[^/]+/[^/]+/pull/[0-9]+' | tail -1 || echo "")
    fi

    if [[ -n "$PR_URL" ]]; then
      REPO_PRS["$repo_num"]="$PR_URL"
    fi
  else
    FAILED=$((FAILED + 1))
    FAILED_REPOS+=("$name")
    REPO_STATUS["$repo_num"]="❌ Failed"

    # Ask if user wants to continue
    if [[ "$FAILED" -lt "$TOTAL" ]]; then
      echo ""
      echo "⚠️  Continue with remaining repositories? (y/n)" >&2
      read -r continue_choice
      if [[ ! "$continue_choice" =~ ^[Yy]$ ]]; then
        echo "Workflow aborted by user"
        break
      fi
    fi
  fi
  rm -f "$SCRIPT_OUTPUT"
done

# Final Summary
echo ""
echo "$SEPARATOR_LINE"
echo "📋 Release Workflow Summary"
echo "$SEPARATOR_LINE"
echo ""
echo "🎯 Version Information:"
echo "   ACM:        $RELEASE_BRANCH"
echo "   Global Hub: release-$GH_VERSION_SHORT"
echo "   OCP:        4.$((OCP_MIN%100)) - 4.$((OCP_MAX%100))"
echo ""
echo "📊 Execution Results: $COMPLETED/$TOTAL completed"
echo ""

# Detailed repository status
echo "📦 Repository Status & Actions:"
echo ""

# Helper function to get repo info
get_repo_display_info() {
  local num=$1
  case $num in
    1) echo "multicluster-global-hub|PR to main|https://github.com/stolostron/multicluster-global-hub/pulls" ;;
    2) echo "openshift/release|PR to master|https://github.com/openshift/release/pulls" ;;
    3) echo "operator-bundle|PR to release-$GH_VERSION_SHORT|https://github.com/stolostron/multicluster-global-hub-operator-bundle/pulls" ;;
    4) echo "operator-catalog|PRs to main & release|https://github.com/stolostron/multicluster-global-hub-operator-catalog/pulls" ;;
    5) echo "glo-grafana|PR to release-$GH_VERSION_SHORT|https://github.com/stolostron/glo-grafana/pulls" ;;
    6) echo "postgres_exporter|PR to $RELEASE_BRANCH|https://github.com/stolostron/postgres_exporter/pulls" ;;
  esac
}

# Display each repository status
for repo_num in $(seq 1 6); do
  if [[ -v REPO_STATUS[$repo_num] ]]; then
    repo_display_info=$(get_repo_display_info "$repo_num")
    IFS='|' read -r repo_name pr_target _ <<< "$repo_display_info"

    status="${REPO_STATUS[$repo_num]}"

    echo "[$repo_num] $repo_name"
    echo "    Status: $status"

    if [[ "$status" == "✅ Completed" ]]; then
      # Only show PR link if we have a specific PR URL
      if [[ -v REPO_PRS[$repo_num] && -n "${REPO_PRS[$repo_num]}" ]]; then
        echo "    Action: Review and merge $pr_target"
        echo "    PR:     ${REPO_PRS[$repo_num]}"
      else
        echo "    Action: No PR created (may already be up to date)"
      fi
    else
      echo "    Action: Check errors above and fix manually"
    fi
    echo ""
  fi
done

# Summary of what to do next
echo "$SEPARATOR_LINE"
if [[ $FAILED -eq 0 ]]; then
  echo "🎉 SUCCESS: All selected repositories updated!"
  echo ""
  echo "📝 Next Steps Checklist:"
  echo ""

  # Count PRs to review (only show repos with actual PRs)
  HAS_PRS=false
  for repo_num in "${EXECUTED_SCRIPTS[@]}"; do
    if [[ -v REPO_PRS[$repo_num] && -n "${REPO_PRS[$repo_num]}" ]]; then
      HAS_PRS=true
      break
    fi
  done

  if [[ "$HAS_PRS" = true ]]; then
    echo "1️⃣  Review and merge PRs:"
    for repo_num in "${EXECUTED_SCRIPTS[@]}"; do
      if [[ -v REPO_PRS[$repo_num] && -n "${REPO_PRS[$repo_num]}" ]]; then
        repo_display_info=$(get_repo_display_info "$repo_num")
        IFS='|' read -r repo_name pr_target _ <<< "$repo_display_info"
        echo "    □ $repo_name - $pr_target"
        echo "      → ${REPO_PRS[$repo_num]}"
      fi
    done
    echo ""
    echo "2️⃣  After PRs are merged:"
    echo "    □ Verify Konflux pipelines are running"
    echo "    □ Check that all images are built successfully"
    echo "    □ Verify release branches are created/updated"
    echo ""
    echo "3️⃣  Manual tasks:"
  else
    echo "ℹ️  No PRs created (repositories may already be up to date)"
    echo ""
    echo "1️⃣  Verify deployment:"
    echo "    □ Check Konflux pipelines are running"
    echo "    □ Verify all images are built successfully"
    echo "    □ Confirm release branches are created/updated"
    echo ""
    echo "2️⃣  Manual tasks:"
  fi
  echo "    □ Update konflux-release-data repository"
  echo "    □ Notify the team about the new release"
  echo ""
  echo "$SEPARATOR_LINE"
  exit 0
else
  echo "⚠️  WARNING: Some repositories failed"
  echo ""
  echo "❌ Failed repositories:"
  for repo in "${FAILED_REPOS[@]}"; do
    echo "   - $repo"
  done
  echo ""
  echo "📝 Action required:"
  echo "   1. Review error messages above"
  echo "   2. Fix issues manually"
  echo "   3. Re-run this script for failed repos only"
  echo ""
  echo "$SEPARATOR_LINE"
  exit 1
fi
