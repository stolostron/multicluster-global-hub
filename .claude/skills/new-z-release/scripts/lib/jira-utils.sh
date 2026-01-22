#!/bin/bash

# Jira utility functions for z-stream release workflow
# NOTE: These are placeholder functions - actual Jira operations
# should be delegated to the jira-administrator agent

# Jira configuration
JIRA_URL="${JIRA_URL:-https://issues.redhat.com}"
JIRA_PROJECT="${JIRA_PROJECT:-ACM}"

# JQL for z-stream release issues based on affected versions
# Usage: build_zstream_issues_jql "v1.5.3"
# This will search for issues affecting v1.5.0, v1.5.1, v1.5.2
build_zstream_issues_jql() {
    local version="$1"

    # Extract version components
    local version_no_v="${version#v}"
    local major="${version_no_v%%.*}"
    local minor="${version_no_v#*.}"
    minor="${minor%%.*}"
    local patch="${version_no_v##*.}"

    # Build list of affected versions to check
    # For v1.5.3, we check v1.5.0, v1.5.1, v1.5.2
    local affected_versions="\"Global Hub ${major}.${minor}.0\""

    # Add previous patch versions
    for ((i=1; i<patch; i++)); do
        affected_versions="${affected_versions}, \"Global Hub ${major}.${minor}.${i}\""
    done

    cat <<EOF
project = ${JIRA_PROJECT}
AND component = "Global Hub"
AND component = security
AND status not in (Closed, Resolved)
AND affectedVersion in (${affected_versions})
EOF
}

# Build Epic title
build_epic_title() {
    local version="$1"
    echo "Global Hub ${version} - Z-Stream Release"
}

# Build Epic labels
build_epic_labels() {
    local version="$1"
    local version_short="${version#v}"

    echo "z-stream,release-${version_short},global-hub"
}

# Build Epic description
build_epic_description() {
    local version="$1"
    local cve_list="$2"
    local base_version=$(echo "$version" | sed -E 's/^v([0-9]+\.[0-9]+)\.[0-9]+$/v\1.0/')
    local version_short="${version#v}"

    cat <<EOF
# Global Hub ${version} Z-Stream Release

## Overview
This Epic tracks the z-stream release for Global Hub ${version}, focusing on critical CVE fixes and high-priority bug resolutions.

## Release Scope

### CVE Fixes
${cve_list}

### Bug Fixes
- Critical bug fixes from previous release
- High-priority customer-reported issues

## Release Information

**Target Version**: ${version}
**Base Version**: ${base_version}
**Release Type**: Z-Stream (Security & Bug Fix)

## Success Criteria

- [ ] All CVE issues resolved and tested
- [ ] All SLA deadlines met
- [ ] Release notes prepared
- [ ] PRs merged to all repositories
- [ ] CI/CD pipelines passing
- [ ] Release published

## Related Links

- Release Branch: https://github.com/stolostron/multicluster-global-hub/tree/release-${version_short%.*}
- Changelog: https://github.com/stolostron/multicluster-global-hub/blob/release-${version_short%.*}/CHANGELOG.md

## Contact

Component Owner: Global Hub Team
EOF
}
