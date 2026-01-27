#!/bin/bash

# Create PRs to onboard z-stream release in both repositories:
# - multicluster-global-hub
# - multicluster-global-hub-operator-bundle

set -euo pipefail

# Configuration
RELEASE_VERSION="${RELEASE_VERSION:-}"
DRY_RUN="${DRY_RUN:-false}"
WORK_DIR="/tmp/globalhub-release"

# Repository URLs
MAIN_REPO_URL="git@github.com:stolostron/multicluster-global-hub.git"
BUNDLE_REPO_URL="git@github.com:stolostron/multicluster-global-hub-operator-bundle.git"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}🚀 Onboard Z-Stream Release - Global Hub ${RELEASE_VERSION:-[VERSION]}${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if [[ -z "$RELEASE_VERSION" ]]; then
    echo -e "${RED}Error: RELEASE_VERSION is required${NC}"
    echo ""
    echo "Usage: RELEASE_VERSION=v1.5.3 $0"
    echo "       DRY_RUN=true RELEASE_VERSION=v1.5.3 $0"
    exit 1
fi

# Extract version components
VERSION_NO_V="${RELEASE_VERSION#v}"
MAJOR="${VERSION_NO_V%%.*}"
MINOR="${VERSION_NO_V#*.}"
MINOR="${MINOR%%.*}"
PATCH="${VERSION_NO_V##*.}"

# Validate it's a z-stream release
if [[ $PATCH -eq 0 ]]; then
    echo -e "${RED}Error: This is not a z-stream release (patch version is 0)${NC}"
    echo "Z-stream releases have patch version > 0 (e.g., v1.5.1, v1.5.2, v1.5.3)"
    exit 1
fi

# Calculate previous version
PREV_PATCH=$((PATCH - 1))
PREV_VERSION="${MAJOR}.${MINOR}.${PREV_PATCH}"

# Calculate release branch
ACM_VERSION=$((MINOR + 9))
RELEASE_BRANCH="release-2.${ACM_VERSION}"
PR_BRANCH="bump-${VERSION_NO_V}"

echo -e "${BOLD}Release Information:${NC}"
echo "  Current Version: ${VERSION_NO_V}"
echo "  Previous Version: ${PREV_VERSION}"
echo "  Main Repo Branch: release-2.${ACM_VERSION}"
echo "  Bundle Repo Branch: release-${MAJOR}.${MINOR}"
echo "  PR Branch: ${PR_BRANCH}"
echo ""

# Check if gh CLI is available
if ! command -v gh &> /dev/null; then
    echo -e "${RED}❌ Error: GitHub CLI (gh) is not installed${NC}"
    echo "Install it with: brew install gh"
    exit 1
fi

# Function to clone or update repository
clone_or_update_repo() {
    local repo_url=$1
    local repo_name=$2
    local repo_path="${WORK_DIR}/${repo_name}"

    echo -e "${BLUE}📦 Setting up ${repo_name}...${NC}"

    if [[ -d "$repo_path" ]]; then
        # Check if it's a valid git repository
        if git -C "$repo_path" rev-parse --git-dir > /dev/null 2>&1; then
            echo -e "${GREEN}  Repository exists, using existing clone${NC}"
            echo -e "${BLUE}  Fetching latest changes...${NC}"
            git -C "$repo_path" fetch origin > /dev/null 2>&1
            echo -e "${GREEN}  ✓ Updated successfully${NC}"
        else
            echo -e "${YELLOW}  Directory exists but not a git repo, removing and re-cloning...${NC}"
            rm -rf "$repo_path"
            echo -e "${BLUE}  Cloning ${repo_url}...${NC}"
            git clone "$repo_url" "$repo_path" > /dev/null 2>&1
            echo -e "${GREEN}  ✓ Cloned successfully${NC}"
        fi
    else
        echo -e "${BLUE}  Cloning ${repo_url}...${NC}"
        git clone "$repo_url" "$repo_path" > /dev/null 2>&1
        echo -e "${GREEN}  ✓ Cloned successfully${NC}"
    fi
    echo ""
}

# Setup work directory
echo -e "${BOLD}Setup:${NC}"
echo "  Work directory: ${WORK_DIR}"
echo ""

mkdir -p "$WORK_DIR"

# Clone repositories
clone_or_update_repo "$MAIN_REPO_URL" "multicluster-global-hub"
clone_or_update_repo "$BUNDLE_REPO_URL" "multicluster-global-hub-operator-bundle"

# Function to update version in a file
update_version() {
    local file=$1
    local old_version=$2
    local new_version=$3

    if [[ ! -f "$file" ]]; then
        echo -e "${RED}❌ File not found: $file${NC}"
        return 1
    fi

    # Try to replace the old_version
    if grep -q "${old_version}" "$file"; then
        sed -i '' "s/${old_version}/${new_version}/g" "$file"
        echo -e "${GREEN}  ✓ Updated $file (from ${old_version})${NC}"
    else
        # If old_version not found, try with older versions (decrement patch)
        local temp_old_version="${old_version}"
        local found=false

        # Extract version components from old_version pattern
        if [[ "$old_version" =~ ([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
            local major="${BASH_REMATCH[1]}"
            local minor="${BASH_REMATCH[2]}"
            local patch="${BASH_REMATCH[3]}"

            # Try decreasing patch versions
            for ((i=patch-1; i>=0; i--)); do
                local try_version="${old_version/${major}.${minor}.${patch}/${major}.${minor}.${i}}"
                if grep -q "${try_version}" "$file"; then
                    sed -i '' "s/${try_version}/${new_version}/g" "$file"
                    echo -e "${GREEN}  ✓ Updated $file (from ${try_version})${NC}"
                    found=true
                    break
                fi
            done
        fi

        if [[ "$found" == "false" ]]; then
            echo -e "${YELLOW}  ⚠ Could not find version to replace in $file${NC}"
        fi
    fi
}

# Function to create PR for a repository
create_pr() {
    local repo_path=$1
    local repo_name=$2
    local RELEASE_BRANCH

    # Calculate release branch based on repository
    if [[ "$repo_name" == "multicluster-global-hub" ]]; then
        # For main repo: v1.5.3 → release-2.14 (minor + 9)
        RELEASE_BRANCH="release-2.${ACM_VERSION}"
    elif [[ "$repo_name" == "multicluster-global-hub-operator-bundle" ]]; then
        # For bundle repo: v1.5.3 → release-1.5 (major.minor)
        RELEASE_BRANCH="release-${MAJOR}.${MINOR}"
    fi

    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    echo -e "${BOLD}Processing: ${repo_name}${NC}"
    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    echo ""

    if [[ ! -d "$repo_path" ]]; then
        echo -e "${RED}❌ Repository not found: $repo_path${NC}"
        return 1
    fi

    cd "$repo_path"

    # Check if we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        echo -e "${RED}❌ Not a git repository: $repo_path${NC}"
        return 1
    fi

    echo -e "${BLUE}📍 Working in: $(pwd)${NC}"
    echo -e "${BLUE}📌 Release Branch: ${RELEASE_BRANCH}${NC}"
    echo ""

    # Ensure we're on the release branch and it's up to date
    echo -e "${BLUE}🔄 Fetching latest changes...${NC}"
    git fetch origin

    # Check if release branch exists
    if ! git rev-parse --verify "origin/${RELEASE_BRANCH}" >/dev/null 2>&1; then
        echo -e "${RED}❌ Release branch ${RELEASE_BRANCH} does not exist on origin${NC}"
        return 1
    fi

    # Checkout release branch
    echo -e "${BLUE}📌 Checking out ${RELEASE_BRANCH}...${NC}"
    if git rev-parse --verify "${RELEASE_BRANCH}" >/dev/null 2>&1; then
        # Local branch exists, just checkout and pull
        git checkout "${RELEASE_BRANCH}"
        git pull origin "${RELEASE_BRANCH}"
    else
        # Local branch doesn't exist, create from origin
        git checkout -b "${RELEASE_BRANCH}" "origin/${RELEASE_BRANCH}"
    fi

    # Create new branch
    echo -e "${BLUE}🌿 Creating branch ${PR_BRANCH}...${NC}"
    if git rev-parse --verify "${PR_BRANCH}" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Branch ${PR_BRANCH} already exists, deleting it...${NC}"
        git branch -D "${PR_BRANCH}"
    fi
    git checkout -b "${PR_BRANCH}"
    echo ""

    # Update files based on repository
    echo -e "${BLUE}📝 Updating version files...${NC}"

    if [[ "$repo_name" == "multicluster-global-hub" ]]; then
        # Update operator/Makefile
        update_version "operator/Makefile" "VERSION ?= ${PREV_VERSION}" "VERSION ?= ${VERSION_NO_V}"

        # Update operator/bundle/manifests CSV
        update_version "operator/bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "name: multicluster-global-hub-operator.v${PREV_VERSION}" \
            "name: multicluster-global-hub-operator.v${VERSION_NO_V}"

        update_version "operator/bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "version: ${PREV_VERSION}" \
            "version: ${VERSION_NO_V}"

        # Calculate version before previous (for replaces field)
        PREV_PREV_PATCH=$((PREV_PATCH - 1))
        PREV_PREV_VERSION="${MAJOR}.${MINOR}.${PREV_PREV_PATCH}"

        update_version "operator/bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "replaces: multicluster-global-hub-operator.v${PREV_PREV_VERSION}" \
            "replaces: multicluster-global-hub-operator.v${PREV_VERSION}"

        # Update operator/config/manifests/bases CSV
        update_version "operator/config/manifests/bases/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "replaces: multicluster-global-hub-operator.v${PREV_PREV_VERSION}" \
            "replaces: multicluster-global-hub-operator.v${PREV_VERSION}"

    elif [[ "$repo_name" == "multicluster-global-hub-operator-bundle" ]]; then
        # Update bundle/manifests CSV
        update_version "bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "name: multicluster-global-hub-operator.v${PREV_VERSION}" \
            "name: multicluster-global-hub-operator.v${VERSION_NO_V}"

        update_version "bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "version: ${PREV_VERSION}" \
            "version: ${VERSION_NO_V}"

        # Calculate version before previous (for replaces field)
        PREV_PREV_PATCH=$((PREV_PATCH - 1))
        PREV_PREV_VERSION="${MAJOR}.${MINOR}.${PREV_PREV_PATCH}"

        update_version "bundle/manifests/multicluster-global-hub-operator.clusterserviceversion.yaml" \
            "replaces: multicluster-global-hub-operator.v${PREV_PREV_VERSION}" \
            "replaces: multicluster-global-hub-operator.v${PREV_VERSION}"
    fi

    echo ""

    # Show diff
    echo -e "${BLUE}📊 Changes made:${NC}"
    git --no-pager diff
    echo ""

    if [[ "$DRY_RUN" == "true" ]]; then
        echo -e "${YELLOW}[DRY RUN] Would commit and create PR${NC}"
        git checkout "${RELEASE_BRANCH}"
        git branch -D "${PR_BRANCH}"
        return 0
    fi

    # Commit changes
    echo -e "${BLUE}💾 Committing changes...${NC}"
    git add .
    git commit -s -m ":sparkles: Bump to ${VERSION_NO_V}"

    # Push branch (force push if branch already exists)
    echo -e "${BLUE}🚀 Pushing branch to origin...${NC}"
    git push -f -u origin "${PR_BRANCH}"

    # Check if PR already exists
    echo -e "${BLUE}📬 Checking for existing pull request...${NC}"
    EXISTING_PR=$(gh pr list --head "${PR_BRANCH}" --json number,url --jq '.[0].url' 2>/dev/null || echo "")

    if [[ -n "$EXISTING_PR" ]]; then
        echo -e "${GREEN}✓ Updated existing pull request!${NC}"
        echo -e "${BLUE}🔗 ${EXISTING_PR}${NC}"
    else
        # Create PR
        echo -e "${BLUE}📬 Creating pull request...${NC}"
        PR_BODY="Bump version to ${VERSION_NO_V} for z-stream release.

## Changes
- Updated version from ${PREV_VERSION} to ${VERSION_NO_V}
- Updated replaces field to reference previous version

## Related
- Part of z-stream release ${RELEASE_VERSION}
"

        PR_URL=$(gh pr create \
            --base "${RELEASE_BRANCH}" \
            --head "${PR_BRANCH}" \
            --title ":sparkles: Bump to ${VERSION_NO_V}" \
            --body "$PR_BODY" \
            2>&1)

        echo -e "${GREEN}✓ Pull request created!${NC}"
        echo -e "${BLUE}🔗 ${PR_URL}${NC}"
    fi
    echo ""

    # Switch back to release branch
    git checkout "${RELEASE_BRANCH}"
}

# Process multicluster-global-hub repository
MAIN_REPO_PATH="${WORK_DIR}/multicluster-global-hub"
create_pr "$MAIN_REPO_PATH" "multicluster-global-hub"

# Process multicluster-global-hub-operator-bundle repository
BUNDLE_REPO_PATH="${WORK_DIR}/multicluster-global-hub-operator-bundle"
create_pr "$BUNDLE_REPO_PATH" "multicluster-global-hub-operator-bundle"

echo -e "${CYAN}════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Onboarding complete!${NC}"
echo -e "${CYAN}════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BOLD}Summary:${NC}"
echo "  Version: ${RELEASE_VERSION}"
echo "  Main Repo Branch: release-2.${ACM_VERSION}"
echo "  Bundle Repo Branch: release-${MAJOR}.${MINOR}"
echo "  PRs created for version bump from ${PREV_VERSION} to ${VERSION_NO_V}"
echo ""
echo -e "${BOLD}Next Steps:${NC}"
echo "  1. Review the PRs and get them merged"
echo "  2. Run: RELEASE_VERSION=${RELEASE_VERSION} scripts/05-konflux-pr.sh"
echo ""

exit 0
