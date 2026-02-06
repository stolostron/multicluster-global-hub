#!/bin/bash

# Create z-stream release Epic and link security issues

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load utilities
source "${SCRIPT_DIR}/lib/jira-utils.sh" 2>/dev/null || true

# Configuration
RELEASE_VERSION="${RELEASE_VERSION:-}"
CVE_LIST="${CVE_LIST:-}"
DRY_RUN="${DRY_RUN:-false}"
PROJECT="${PROJECT:-ACM}"
COMPONENT="${COMPONENT:-Global Hub}"

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
echo -e "${BOLD}📝 Create Z-Stream Release Epic - Global Hub ${RELEASE_VERSION:-[VERSION]}${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if [[ -z "$RELEASE_VERSION" ]]; then
    echo -e "${RED}Error: RELEASE_VERSION is required${NC}"
    echo ""
    echo "Usage: RELEASE_VERSION=v1.5.3 $0"
    echo "       RELEASE_VERSION=v1.5.3 CVE_LIST=\"ACM-28186,ACM-28100\" $0"
    echo "       DRY_RUN=true RELEASE_VERSION=v1.5.3 $0"
    exit 1
fi

# Extract version components
VERSION_NO_V="${RELEASE_VERSION#v}"
MAJOR="${VERSION_NO_V%%.*}"
MINOR="${VERSION_NO_V#*.}"
MINOR="${MINOR%%.*}"
PATCH="${VERSION_NO_V##*.}"

# Calculate versions
# For z-stream v1.5.3, affected version should be previous patch (1.5.2)
BASE_VERSION="v${MAJOR}.${MINOR}.0"
PREV_PATCH=$((PATCH - 1))
AFFECTED_VERSION_NO_V="${MAJOR}.${MINOR}.${PREV_PATCH}"
FIX_VERSION_NO_V="${VERSION_NO_V}"
ACM_VERSION=$((MINOR + 9))

# Epic details
EPIC_TITLE=$(build_epic_title "$RELEASE_VERSION")
EPIC_LABELS=$(build_epic_labels "$RELEASE_VERSION")

echo -e "${BOLD}Release Information:${NC}"
echo "  Version: ${RELEASE_VERSION}"
echo "  Base Version: ${BASE_VERSION}"
echo "  Fix Version: Global Hub ${FIX_VERSION_NO_V}"
echo "  Affected Version: Global Hub ${AFFECTED_VERSION_NO_V}"
echo "  ACM Version: 2.${ACM_VERSION}.z"
echo ""

# Auto-discover CVEs if not provided
if [[ -z "$CVE_LIST" ]]; then
    echo -e "${BLUE}🔍 Auto-discovering CVE issues...${NC}"

    # Check if CVE discovery results exist
    CVE_FILE="/tmp/globalhub-${VERSION_NO_V}-cves.txt"
    if [[ -f "$CVE_FILE" ]]; then
        echo -e "${GREEN}✓ Found CVE discovery results from previous run${NC}"
        # Extract issue keys from the saved file (skip header lines)
        CVE_LIST=$(tail -n +5 "$CVE_FILE" | awk '{print $1}' | grep "^ACM-" | tr '\n' ',' | sed 's/,$//')
    else
        echo -e "${YELLOW}⚠️  No CVE discovery results found${NC}"
        echo "Run: RELEASE_VERSION=${RELEASE_VERSION} .claude/skills/new-z-release/scripts/01-discover-cves.sh"
        echo ""
        echo "Or provide CVE_LIST manually:"
        echo "  RELEASE_VERSION=${RELEASE_VERSION} CVE_LIST=\"ACM-123,ACM-456\" $0"
        exit 1
    fi
    echo ""
fi

# Parse CVE list
IFS=',' read -ra CVE_ARRAY <<< "$CVE_LIST"
CVE_COUNT=${#CVE_ARRAY[@]}

echo -e "${BOLD}Epic Details:${NC}"
echo "  Project: ${PROJECT}"
echo "  Title: ${EPIC_TITLE}"
echo "  Component: ${COMPONENT}"
echo "  Labels: GlobalHub"
echo "  Activity Type: Security & Compliance"
echo "  Priority: High"
echo "  Affected Version: Global Hub ${AFFECTED_VERSION_NO_V}"
echo "  Fix Version: Global Hub ${FIX_VERSION_NO_V}"
echo ""

if [[ ${CVE_COUNT} -gt 0 ]]; then
    echo -e "${BOLD}CVE Issues to Link (${CVE_COUNT}):${NC}"
    for cve in "${CVE_ARRAY[@]}"; do
        echo "  - ${cve}"
    done
    echo ""
fi

# Build CVE list for Epic description
CVE_LIST_FORMATTED=""
if [[ ${CVE_COUNT} -gt 0 ]]; then
    for cve in "${CVE_ARRAY[@]}"; do
        CVE_LIST_FORMATTED="${CVE_LIST_FORMATTED}- [${cve}|https://issues.redhat.com/browse/${cve}]
"
    done
else
    CVE_LIST_FORMATTED="- No CVE issues linked (will be added later)
"
fi

# Build Epic description in Jira Wiki Markup format
EPIC_DESCRIPTION="h2. Overview

This Epic tracks the z-stream release for Global Hub ${RELEASE_VERSION}, focusing on critical CVE fixes and high-priority bug resolutions.

h2. Release Scope

h3. CVE Fixes

${CVE_LIST_FORMATTED}
h3. Bug Fixes

* Critical bug fixes from previous release
* High-priority customer-reported issues

h2. Release Information

* *Target Version*: ${RELEASE_VERSION}
* *Base Version*: ${BASE_VERSION}
* *Release Type*: Z-Stream (Security & Bug Fix)
* *ACM Version*: 2.${ACM_VERSION}.z

h2. Success Criteria

* All CVE issues resolved and tested
* All SLA deadlines met
* Release notes prepared
* PRs merged to all repositories
* CI/CD pipelines passing
* Release published

h2. Related Links

* [Release Branch|https://github.com/stolostron/multicluster-global-hub/tree/release-${MAJOR}.${MINOR}]
* [Changelog|https://github.com/stolostron/multicluster-global-hub/blob/release-${MAJOR}.${MINOR}/CHANGELOG.md]

h2. Contact

*Component Owner*: Global Hub Team

---
_Auto-generated for ${RELEASE_VERSION} z-stream release_"

# Preview mode
if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    echo -e "${YELLOW}[DRY RUN] Epic Description Preview:${NC}"
    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    echo ""
    echo "$EPIC_DESCRIPTION"
    echo ""
    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    echo -e "${YELLOW}[DRY RUN] Would create Epic with above details${NC}"
    echo -e "${CYAN}────────────────────────────────────────────────${NC}"
    exit 0
fi

# Check if jira CLI is available
if ! command -v jira &> /dev/null; then
    echo -e "${RED}❌ Error: Jira CLI is not installed${NC}"
    echo "Install it with: brew install jira-cli"
    exit 1
fi

echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Creating Epic${NC}"
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo ""

# Create Epic using jira CLI
echo -e "${BLUE}🚀 Creating Epic...${NC}"

# Set activity type in a variable to avoid issues with special characters
ACTIVITY_TYPE="Security & Compliance"
AFFECTED_VERSION="Global Hub ${AFFECTED_VERSION_NO_V}"
FIX_VERSION="Global Hub ${FIX_VERSION_NO_V}"
ASSIGNEE=$(jira me)

echo "Debug: About to run jira issue create command..."
echo "Debug: Project=$PROJECT, Type=Epic, Assignee=$ASSIGNEE"

# Capture full output first for debugging
set +e  # Don't exit on error
EPIC_OUTPUT=$(jira epic create \
    --project "$PROJECT" \
    --name "$EPIC_TITLE" \
    --summary "$EPIC_TITLE" \
    --body "$EPIC_DESCRIPTION" \
    --component "$COMPONENT" \
    --label "GlobalHub" \
    --affects-version "$AFFECTED_VERSION" \
    --fix-version "$FIX_VERSION" \
    --custom activity-type="${ACTIVITY_TYPE}" \
    --priority Critical \
    --assignee "$ASSIGNEE" \
    --no-input 2>&1)
JIRA_EXIT_CODE=$?
set -e  # Re-enable exit on error

echo "Debug: Jira command completed with exit code: $JIRA_EXIT_CODE"
echo "Debug: Output length: ${#EPIC_OUTPUT} characters"
echo "Debug: Raw output:"
echo "$EPIC_OUTPUT"
echo ""

EPIC_KEY=$(echo "$EPIC_OUTPUT" | grep -oE "${PROJECT}-[0-9]+" | head -1)
echo "Debug: Extracted EPIC_KEY='$EPIC_KEY'"

if [[ -z "$EPIC_KEY" ]]; then
    echo -e "${RED}❌ Failed to create Epic${NC}"
    echo ""
    echo "Exit Code: $JIRA_EXIT_CODE"
    echo ""
    echo "Jira CLI output:"
    echo "$EPIC_OUTPUT"
    echo ""
    echo "Please check if the version names exist in Jira:"
    echo "  - Affected Version: $AFFECTED_VERSION"
    echo "  - Fix Version: $FIX_VERSION"
    exit 1
fi

echo -e "${GREEN}✓ Epic created: ${EPIC_KEY}${NC}"
echo -e "${BLUE}🔗 ${GREEN}https://issues.redhat.com/browse/${EPIC_KEY}${NC}"
echo ""

# Add CVE issues to Epic
if [[ ${CVE_COUNT} -gt 0 ]]; then
    echo -e "${BLUE}🔗 Adding ${CVE_COUNT} CVE issue(s) to Epic...${NC}"
    echo ""

    # Use jira epic add with all issues at once
    # Syntax: jira epic add EPIC-KEY ISSUE-1 ISSUE-2 ISSUE-3 ...
    if jira epic add "$EPIC_KEY" "${CVE_ARRAY[@]}" 2>&1; then
        echo -e "${GREEN}✓ Successfully added all ${CVE_COUNT} CVE issues to Epic${NC}"
    else
        echo -e "${RED}✗ Failed to add CVE issues to Epic${NC}"
        echo "  Please manually add them in Jira or run:"
        echo "  jira epic add $EPIC_KEY ${CVE_ARRAY[@]}"
    fi
    echo ""
fi

echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Summary${NC}"
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo ""
echo -e "${GREEN}✓ Epic created successfully!${NC}"
echo ""
echo -e "${BOLD}Epic:${NC} ${EPIC_KEY}"
echo -e "${BOLD}URL:${NC} https://issues.redhat.com/browse/${EPIC_KEY}"
echo -e "${BOLD}Version:${NC} ${RELEASE_VERSION}"
echo -e "${BOLD}CVE Issues:${NC} ${CVE_COUNT}"
echo ""

# Display the created Epic
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Epic Details${NC}"
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo ""
jira issue view "${EPIC_KEY}"
echo ""

echo -e "${BOLD}Next Steps:${NC}"
echo "  1. Review the Epic in Jira"
echo "  2. Verify all CVE issues are linked correctly"
echo "  3. Run: RELEASE_VERSION=${RELEASE_VERSION} scripts/04-onboard-release.sh"
echo ""

exit 0
