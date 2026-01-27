#!/bin/bash

# GitHub utility functions for z-stream release workflow

# GitHub configuration
GITHUB_ORG="${GITHUB_ORG:-stolostron}"
GITHUB_REPO_MAIN="${GITHUB_REPO_MAIN:-multicluster-global-hub}"
GITHUB_REPO_KONFLUX="${GITHUB_REPO_KONFLUX:-konflux-release-data}"

# Get release branch for version
get_release_branch() {
    local version="$1"
    local version_no_v="${version#v}"
    local minor="${version_no_v#*.}"
    minor="${minor%%.*}"

    # Global Hub minor + 9 = ACM version
    local acm_version=$((minor + 9))

    echo "release-2.${acm_version}"
}

# Build PR title for z-stream release
build_pr_title() {
    local version="$1"
    echo "chore: Onboard Global Hub ${version} z-stream release"
}

# Build PR body for main repo
build_main_repo_pr_body() {
    local version="$1"
    local cve_list="$2"

    cat <<EOF
## Description

Onboard Global Hub ${version} z-stream release.

## Changes

- Update VERSION to ${version}
- Update CHANGELOG.md with CVE fixes
- Update operator/manager/agent version metadata
- Update bundle version information

## CVE Fixes

${cve_list}

## Testing

- [ ] Local build successful
- [ ] Bundle generation successful
- [ ] CI pipelines passing

## Checklist

- [ ] VERSION file updated
- [ ] CHANGELOG.md updated with all CVE fixes
- [ ] Operator version metadata updated
- [ ] Manager version updated
- [ ] Agent version updated
- [ ] Bundle regenerated with \`make bundle\`
- [ ] All tests passing

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
}
