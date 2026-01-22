#!/bin/bash

# Discover CVE and security issues for z-stream release
# This script finds all security issues affecting a specific Global Hub version

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load utilities
source "${SCRIPT_DIR}/lib/jira-utils.sh" 2>/dev/null || true

# Configuration
RELEASE_VERSION="${RELEASE_VERSION:-}"
DRY_RUN="${DRY_RUN:-false}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

# Print banner
echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}📋 CVE Discovery - Global Hub ${RELEASE_VERSION:-[VERSION]}${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if [[ -z "$RELEASE_VERSION" ]]; then
    echo -e "${RED}Error: RELEASE_VERSION is required${NC}"
    echo ""
    echo "Usage: RELEASE_VERSION=v1.5.3 $0"
    exit 1
fi

# Extract version components
VERSION_NO_V="${RELEASE_VERSION#v}"
MAJOR="${VERSION_NO_V%%.*}"
MINOR="${VERSION_NO_V#*.}"
MINOR="${MINOR%%.*}"
PATCH="${VERSION_NO_V##*.}"

echo -e "${BOLD}Release Information:${NC}"
echo "  Version: ${RELEASE_VERSION}"
echo "  Major: ${MAJOR}, Minor: ${MINOR}, Patch: ${PATCH}"
echo ""

# Build affected versions list
AFFECTED_VERSIONS=""
for ((i=0; i<PATCH; i++)); do
    if [[ -n "$AFFECTED_VERSIONS" ]]; then
        AFFECTED_VERSIONS="${AFFECTED_VERSIONS}, "
    fi
    AFFECTED_VERSIONS="${AFFECTED_VERSIONS}\"Global Hub ${MAJOR}.${MINOR}.${i}\""
done

echo -e "${BOLD}Affected Versions to Check:${NC}"
echo "  ${AFFECTED_VERSIONS}"
echo ""

# Build JQL query
JQL=$(build_zstream_issues_jql "$RELEASE_VERSION")

# Convert multi-line JQL to single line for jira CLI
JQL_SINGLE_LINE=$(echo "$JQL" | tr '\n' ' ')

echo -e "${BOLD}JQL Query:${NC}"
echo "${JQL}"
echo ""

if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${YELLOW}[DRY RUN] Would execute CVE discovery${NC}"
    exit 0
fi

# Check if jira CLI is available
if ! command -v jira &> /dev/null; then
    echo -e "${RED}❌ Error: Jira CLI is not installed${NC}"
    echo "Install it with: brew install jira-cli"
    echo ""
    echo "Or run this query manually in Jira:"
    echo "  https://issues.redhat.com/issues/?jql=${JQL// /%20}"
    exit 1
fi

echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Executing CVE Discovery${NC}"
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo ""

# Execute JQL query using jira CLI
echo -e "${BLUE}🔍 Searching for security issues...${NC}"
echo ""

# Get issues matching the JQL
ISSUES=$(jira issue list --jql "$JQL_SINGLE_LINE" --plain --columns KEY,SUMMARY,STATUS,PRIORITY,ASSIGNEE,CREATED 2>&1)
JIRA_EXIT_CODE=$?

if [[ $JIRA_EXIT_CODE -ne 0 ]]; then
    echo -e "${RED}❌ Error executing jira CLI:${NC}"
    echo "$ISSUES"
    echo ""
    echo "Try running this query manually in Jira:"
    echo "  https://issues.redhat.com/issues/"
    echo ""
    echo "Or check jira CLI authentication:"
    echo "  jira init"
    exit 1
fi

if [[ -z "$ISSUES" ]] || [[ "$ISSUES" == *"No result"* ]]; then
    echo -e "${YELLOW}ℹ️  No open security issues found for ${RELEASE_VERSION}${NC}"
    echo ""
    echo "This could mean:"
    echo "  • All security issues are already Closed or Resolved"
    echo "  • Issues don't have both 'Global Hub' and 'security' components"
    echo "  • Affected Version field is not set correctly"
    echo ""
    exit 0
fi

# Count issues
ISSUE_COUNT=$(echo "$ISSUES" | tail -n +2 | wc -l | tr -d ' ')

echo -e "${GREEN}✓ Found ${ISSUE_COUNT} security issue(s)${NC}"
echo ""

# Get detailed info for each issue including SLA dates
echo -e "${BLUE}📅 Fetching SLA information...${NC}"
echo ""

# Parse issues into temporary file to avoid stdin conflicts
TEMP_ISSUES=$(mktemp)
echo "$ISSUES" | tail -n +2 > "$TEMP_ISSUES"

# Display issues with SLA highlighting
echo -e "${BOLD}Security Issues for ${RELEASE_VERSION} (with SLA Dates):${NC}"
echo ""
printf "%-12s %-15s %-25s %s\n" "KEY" "SLA DATE" "URGENCY" "SUMMARY"
echo "──────────────────────────────────────────────────────────────────────────────────────────────────────"

CURRENT_DATE=$(date +%s)
OVERDUE_COUNT=0
CRITICAL_COUNT=0
WARNING_COUNT=0
ISSUE_KEYS=""
OUTPUT_DETAILS=""

# Process each issue
while IFS=$'\t' read -r KEY SUMMARY REST; do
    # Add to issue keys list
    if [[ -n "$ISSUE_KEYS" ]]; then
        ISSUE_KEYS="${ISSUE_KEYS},${KEY}"
    else
        ISSUE_KEYS="${KEY}"
    fi

    # Get issue details for SLA date
    ISSUE_VIEW=$(jira issue view "$KEY" 2>&1 < /dev/null | head -3)

    # Extract SLA date from header (format: "⌛ Fri, 09 Jan 26")
    SLA_DATE=$(echo "$ISSUE_VIEW" | grep -o '⌛[^👷]*' | sed 's/⌛[[:space:]]*//' | xargs)

    # Truncate summary to 80 chars
    if [[ ${#SUMMARY} -gt 80 ]]; then
        SUMMARY="${SUMMARY:0:77}..."
    fi

    if [[ -z "$SLA_DATE" ]]; then
        printf "%-12s %-15s %-25s %s\n" "$KEY" "N/A" "-" "$SUMMARY"
        OUTPUT_DETAILS="${OUTPUT_DETAILS}${KEY}\tN/A\t-\t${SUMMARY}\n"
    else
        # Parse date and calculate days remaining
        SLA_EPOCH=$(date -j -f "%a, %d %b %y" "$SLA_DATE" +%s 2>/dev/null || echo "0")

        if [[ "$SLA_EPOCH" != "0" ]]; then
            DAYS_REMAINING=$(( (SLA_EPOCH - CURRENT_DATE) / 86400 ))

            # Determine urgency and color
            if [[ $DAYS_REMAINING -lt 0 ]]; then
                URGENCY="${RED}🔴 OVERDUE${NC}"
                URGENCY_PLAIN="OVERDUE"
                ((OVERDUE_COUNT++))
            elif [[ $DAYS_REMAINING -le 3 ]]; then
                URGENCY="${RED}⚠️  CRITICAL (${DAYS_REMAINING}d)${NC}"
                URGENCY_PLAIN="CRITICAL (${DAYS_REMAINING}d)"
                ((CRITICAL_COUNT++))
            elif [[ $DAYS_REMAINING -le 7 ]]; then
                URGENCY="${YELLOW}⚡ WARNING (${DAYS_REMAINING}d)${NC}"
                URGENCY_PLAIN="WARNING (${DAYS_REMAINING}d)"
                ((WARNING_COUNT++))
            else
                URGENCY="${GREEN}✓ OK (${DAYS_REMAINING}d)${NC}"
                URGENCY_PLAIN="OK (${DAYS_REMAINING}d)"
            fi

            printf "%-12s %-15s " "$KEY" "$SLA_DATE"
            echo -e "${URGENCY} ${SUMMARY}"
            OUTPUT_DETAILS="${OUTPUT_DETAILS}${KEY}\t${SLA_DATE}\t${URGENCY_PLAIN}\t${SUMMARY}\n"
        else
            printf "%-12s %-15s %-25s %s\n" "$KEY" "$SLA_DATE" "?" "$SUMMARY"
            OUTPUT_DETAILS="${OUTPUT_DETAILS}${KEY}\t${SLA_DATE}\t?\t${SUMMARY}\n"
        fi
    fi
done < "$TEMP_ISSUES"

rm -f "$TEMP_ISSUES"
echo ""

echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo -e "${BOLD}Summary${NC}"
echo -e "${CYAN}────────────────────────────────────────────────${NC}"
echo ""
echo -e "${BOLD}Total Issues:${NC} ${ISSUE_COUNT}"

if [[ $OVERDUE_COUNT -gt 0 ]]; then
    echo -e "${RED}  🔴 Overdue: ${OVERDUE_COUNT}${NC}"
fi
if [[ $CRITICAL_COUNT -gt 0 ]]; then
    echo -e "${RED}  ⚠️  Critical (≤3 days): ${CRITICAL_COUNT}${NC}"
fi
if [[ $WARNING_COUNT -gt 0 ]]; then
    echo -e "${YELLOW}  ⚡ Warning (≤7 days): ${WARNING_COUNT}${NC}"
fi

echo ""
echo -e "${BOLD}Issue Keys:${NC} ${ISSUE_KEYS}"
echo ""
echo -e "${BOLD}Next Steps:${NC}"
echo "  1. Review the issues above (focus on OVERDUE and CRITICAL items)"
echo "  2. Run: RELEASE_VERSION=${RELEASE_VERSION} CVE_LIST=\"${ISSUE_KEYS}\" scripts/03-create-epic.sh"
echo "  3. Or manually: jira issue view <ISSUE-KEY> for details"
echo ""

# Save results to file with SLA info
OUTPUT_FILE="/tmp/globalhub-${VERSION_NO_V}-cves.txt"
{
    echo "Security Issues for Global Hub ${RELEASE_VERSION}"
    echo "Generated: $(date)"
    echo ""
    printf "%-12s %-15s %-25s %s\n" "KEY" "SLA DATE" "URGENCY" "SUMMARY"
    echo "──────────────────────────────────────────────────────────────────────────────────────────────────────────"
    echo -e "$OUTPUT_DETAILS"
} > "$OUTPUT_FILE"

echo -e "${BLUE}💾 Results saved to: ${OUTPUT_FILE}${NC}"
echo ""

exit 0
