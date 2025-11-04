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
#   CUT_MODE=true RELEASE_BRANCH=release-2.17 ./cut-release.sh all  # Cut mode - create and push release branches directly to upstream
#
# Note: RELEASE_BRANCH environment variable is REQUIRED (e.g., release-2.14, release-2.15, release-2.16, release-2.17)

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
  case $num in
    1) echo "$REPO_1" ;;
    2) echo "$REPO_2" ;;
    3) echo "$REPO_3" ;;
    4) echo "$REPO_4" ;;
    5) echo "$REPO_5" ;;
    6) echo "$REPO_6" ;;
    *) echo "" ;;
  esac
}

# Parse command line argument
MODE="${1:-interactive}"

# Configuration
RELEASE_BRANCH="${RELEASE_BRANCH:-}"
OPENSHIFT_RELEASE_PATH="${OPENSHIFT_RELEASE_PATH:-/tmp/openshift-release}"
CUT_MODE="${CUT_MODE:-false}"

# Auto-detect GitHub user from current git repo (can be overridden with GITHUB_USER env var)
if [ -z "${GITHUB_USER:-}" ]; then
  GITHUB_USER=$(git remote get-url origin 2>/dev/null | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|' || echo "")
  if [ -z "$GITHUB_USER" ]; then
    echo "❌ Error: Could not auto-detect GitHub user from git remote"
    echo "   Please set GITHUB_USER environment variable or run from a git repository"
    exit 1
  fi
fi

export GITHUB_USER
export CUT_MODE

# RELEASE_BRANCH must be explicitly specified
if [ -z "$RELEASE_BRANCH" ]; then
  echo "❌ Error: RELEASE_BRANCH environment variable is required"
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
ACM_VERSION=$(echo "$RELEASE_BRANCH" | sed 's/release-//')
ACM_MAJOR=$(echo "$ACM_VERSION" | cut -d. -f1)
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
echo "================================================"
echo "   Mode:        $([ "$CUT_MODE" = true ] && echo "CUT (create branches)" || echo "UPDATE (PR only)")"
echo "   GitHub User: $GITHUB_USER"
echo "   ACM:         $RELEASE_BRANCH"
echo "   Global Hub:  release-$GH_VERSION_SHORT"
echo "   Bundle:      release-$GH_VERSION_SHORT"
echo "   Catalog:     release-$GH_VERSION_SHORT"
echo "   OCP:         4.${OCP_MIN} - 4.${OCP_MAX}"
echo "================================================"
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
    local repo_info=$(get_repo_info "$i")
    IFS='|' read -r name script desc <<< "$repo_info"
    printf "   [%d] %-25s %s\n" "$i" "$name" "$desc"
  done
  echo ""
}

# Function to run a specific script
run_script() {
  local repo_num=$1
  local repo_info=$(get_repo_info "$repo_num")
  IFS='|' read -r name script desc <<< "$repo_info"

  echo ""
  echo "────────────────────────────────────────────────"
  echo "📦 [$repo_num/6] $name"
  echo "────────────────────────────────────────────────"
  echo "   $desc"
  echo ""

  if [ ! -f "$SCRIPT_DIR/$script" ]; then
    echo "❌ Error: Script not found: $script"
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
    echo "❌ $name failed"
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

    if [ -z "$selection" ]; then
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
echo "================================================"
echo ""

# Track results
TOTAL=${#REPOS_TO_UPDATE[@]}
COMPLETED=0
FAILED=0
FAILED_REPOS=()

# Execute selected scripts
for repo_num in "${REPOS_TO_UPDATE[@]}"; do
  # Validate repo number
  if [ "$repo_num" -lt 1 ] || [ "$repo_num" -gt 6 ]; then
    echo "⚠️  Invalid repository number: $repo_num (skipping)"
    continue
  fi

  if run_script "$repo_num"; then
    COMPLETED=$((COMPLETED + 1))
  else
    FAILED=$((FAILED + 1))
    repo_info=$(get_repo_info "$repo_num")
    IFS='|' read -r name _ _ <<< "$repo_info"
    FAILED_REPOS+=("$name")

    # Ask if user wants to continue
    if [ $FAILED -lt $TOTAL ]; then
      echo ""
      echo "⚠️  Continue with remaining repositories? (y/n)"
      read -r continue_choice
      if [[ ! "$continue_choice" =~ ^[Yy]$ ]]; then
        echo "Workflow aborted by user"
        break
      fi
    fi
  fi
done

# Final Summary
echo ""
echo "================================================"
echo "📋 Release Workflow Summary"
echo "================================================"
echo ""
echo "Version:"
echo "   ACM:        $RELEASE_BRANCH"
echo "   Global Hub: release-$GH_VERSION_SHORT"
echo ""
echo "Results:"
echo "   Total: $TOTAL"
echo "   ✅ Completed: $COMPLETED"
echo "   ❌ Failed: $FAILED"

if [ $FAILED -gt 0 ]; then
  echo ""
  echo "Failed repositories:"
  for repo in "${FAILED_REPOS[@]}"; do
    echo "   - $repo"
  done
fi

echo ""
echo "================================================"

if [ $FAILED -eq 0 ]; then
  echo "🎉 All selected repositories updated successfully!"
  echo ""
  echo "📝 Next Steps:"
  echo "   1. Review and merge created PRs"
  echo "   2. Verify all release branches"
  echo "   3. Update konflux-release-data (manual)"
  echo ""
  exit 0
else
  echo "⚠️  Some repositories failed to update"
  echo ""
  echo "Please review errors above and fix manually."
  echo ""
  exit 1
fi
