#!/bin/bash

# Create MR to konflux-release-data for z-stream release

set -euo pipefail

# Configuration
RELEASE_VERSION="${RELEASE_VERSION:-}"
DRY_RUN="${DRY_RUN:-false}"
KONFLUX_REPO_PATH="${KONFLUX_REPO_PATH:-../../../gitlab.cee.redhat.com/konflux-release-data}"

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
echo -e "${BOLD}📦 Konflux Release Data MR - Global Hub ${RELEASE_VERSION:-[VERSION]}${NC}"
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

# Calculate file path components
VERSION_DASH="${MAJOR}-${MINOR}"  # e.g., 1-5 for v1.5.3
MR_BRANCH="bump-globalhub-${VERSION_NO_V}"

# Config file paths
CONFIG_DIR="config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/acm-multicluster-glo"
PROD_FILE="${CONFIG_DIR}/acm-multicluster-glo-${VERSION_DASH}-rpa-prod.yaml"
STAGE_FILE="${CONFIG_DIR}/acm-multicluster-glo-${VERSION_DASH}-rpa-stage.yaml"

echo -e "${BOLD}Release Information:${NC}"
echo "  Current Version: ${VERSION_NO_V}"
echo "  Previous Version: ${PREV_VERSION}"
echo "  Version Dash Format: ${VERSION_DASH}"
echo "  MR Branch: ${MR_BRANCH}"
echo ""

echo -e "${BOLD}Files to Update:${NC}"
echo "  - ${PROD_FILE}"
echo "  - ${STAGE_FILE}"
echo ""

# Check if konflux repo exists
if [[ ! -d "$KONFLUX_REPO_PATH" ]]; then
    echo -e "${RED}❌ Konflux repo not found: $KONFLUX_REPO_PATH${NC}"
    exit 1
fi

# Save current directory
ORIGINAL_DIR="$(pwd)"

cd "$KONFLUX_REPO_PATH"

echo -e "${BLUE}📍 Working in: $(pwd)${NC}"
echo ""

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}❌ Not a git repository: $KONFLUX_REPO_PATH${NC}"
    exit 1
fi

# Fetch and pull latest changes
echo -e "${BLUE}🔄 Fetching latest changes from origin...${NC}"
git fetch origin
git checkout main
git pull origin main

# Check if files exist
if [[ ! -f "$PROD_FILE" ]]; then
    echo -e "${RED}❌ Production file not found: $PROD_FILE${NC}"
    exit 1
fi

if [[ ! -f "$STAGE_FILE" ]]; then
    echo -e "${RED}❌ Stage file not found: $STAGE_FILE${NC}"
    exit 1
fi

# Create new branch
echo -e "${BLUE}🌿 Creating branch ${MR_BRANCH}...${NC}"
if git rev-parse --verify "${MR_BRANCH}" >/dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  Branch ${MR_BRANCH} already exists, deleting it...${NC}"
    git branch -D "${MR_BRANCH}"
fi
git checkout -b "${MR_BRANCH}"
echo ""

# Update files
echo -e "${BLUE}📝 Updating version files...${NC}"

# Update production file
sed -i '' "s/product_version: \"${PREV_VERSION}\"/product_version: \"${VERSION_NO_V}\"/g" "$PROD_FILE"
sed -i '' "s/- \"${PREV_VERSION}\"/- \"${VERSION_NO_V}\"/g" "$PROD_FILE"
sed -i '' "s/- \"${PREV_VERSION}-{{ timestamp }}\"/- \"${VERSION_NO_V}-{{ timestamp }}\"/g" "$PROD_FILE"
echo -e "${GREEN}  ✓ Updated ${PROD_FILE}${NC}"

# Update stage file
sed -i '' "s/product_version: \"${PREV_VERSION}\"/product_version: \"${VERSION_NO_V}\"/g" "$STAGE_FILE"
sed -i '' "s/- \"${PREV_VERSION}\"/- \"${VERSION_NO_V}\"/g" "$STAGE_FILE"
sed -i '' "s/- \"${PREV_VERSION}-{{ timestamp }}\"/- \"${VERSION_NO_V}-{{ timestamp }}\"/g" "$STAGE_FILE"
echo -e "${GREEN}  ✓ Updated ${STAGE_FILE}${NC}"

echo ""

# Show diff
echo -e "${BLUE}📊 Changes made:${NC}"
git diff
echo ""

if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${YELLOW}[DRY RUN] Would commit and create MR${NC}"
    git checkout main
    git branch -D "${MR_BRANCH}"
    cd "$ORIGINAL_DIR"
    exit 0
fi

# Commit changes
echo -e "${BLUE}💾 Committing changes...${NC}"
git add "$PROD_FILE" "$STAGE_FILE"
git commit -s -m "Bump Global Hub to ${VERSION_NO_V}

Update multicluster-global-hub version from ${PREV_VERSION} to ${VERSION_NO_V}

Changes:
- Updated product_version in prod and stage RPA files
- Updated version tags in defaults

Related: z-stream release ${RELEASE_VERSION}
"

# Push branch
echo -e "${BLUE}🚀 Pushing branch to origin...${NC}"
git push -u origin "${MR_BRANCH}"

# Create MR using GitLab CLI
echo -e "${BLUE}📬 Creating merge request...${NC}"

# Check if glab CLI is available
if ! command -v glab &> /dev/null; then
    echo -e "${YELLOW}⚠️  GitLab CLI (glab) not found${NC}"
    echo ""
    echo "Please create MR manually:"
    echo "  https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/new?merge_request[source_branch]=${MR_BRANCH}"
    cd "$ORIGINAL_DIR"
    exit 0
fi

MR_TITLE="Bump Global Hub to ${VERSION_NO_V}"
MR_DESCRIPTION="Update multicluster-global-hub version from ${PREV_VERSION} to ${VERSION_NO_V}

## Changes
- Updated \`product_version\` in prod and stage RPA files
- Updated version tags in defaults

## Files Modified
- ${PROD_FILE}
- ${STAGE_FILE}

## Related
- Z-stream release ${RELEASE_VERSION}
"

MR_URL=$(glab mr create \
    --title "$MR_TITLE" \
    --description "$MR_DESCRIPTION" \
    --target-branch main \
    --source-branch "${MR_BRANCH}" \
    2>&1 | grep -oE 'https://[^ ]+' | head -1)

if [[ -n "$MR_URL" ]]; then
    echo -e "${GREEN}✓ Merge request created!${NC}"
    echo -e "${BLUE}🔗 ${MR_URL}${NC}"
else
    echo -e "${YELLOW}⚠️  Could not extract MR URL${NC}"
    echo "Please check GitLab for the merge request"
fi
echo ""

# Switch back to main
git checkout main

# Return to original directory
cd "$ORIGINAL_DIR"

echo -e "${CYAN}════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Konflux MR complete!${NC}"
echo -e "${CYAN}════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BOLD}Summary:${NC}"
echo "  Version: ${RELEASE_VERSION}"
echo "  MR Branch: ${MR_BRANCH}"
echo "  Files Updated: prod and stage RPA files"
echo ""

exit 0
