#!/bin/bash

# Create PR to konflux-release-data for z-stream release

set -euo pipefail

# Configuration
RELEASE_VERSION="${RELEASE_VERSION:-}"

# Colors
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}📦 Konflux Release Data PR - Global Hub ${RELEASE_VERSION:-[VERSION]}${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if [[ -z "$RELEASE_VERSION" ]]; then
    echo -e "${RED}Error: RELEASE_VERSION is required${NC}"
    echo ""
    echo "Usage: RELEASE_VERSION=v1.5.3 $0"
    exit 1
fi

echo -e "${BOLD}Release Information:${NC}"
echo "  Version: ${RELEASE_VERSION}"
echo "  Repository: stolostron/konflux-release-data"
echo ""

echo -e "${YELLOW}NOTE:${NC} Use Claude Code to create the konflux PR."
echo ""
echo "Request: 'Create konflux-release-data PR for Global Hub ${RELEASE_VERSION}'"
echo ""

exit 0
