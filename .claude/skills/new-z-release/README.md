# Global Hub Z-Stream Release Skill

Automates z-stream release workflow for Multicluster Global Hub, including CVE tracking, SLA monitoring, Epic management, and release onboarding.

## Quick Start

### Individual Commands

Each sub-command can be run independently:

```bash
# 1. Discover CVE issues with SLA monitoring
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/01-discover-cves.sh

# 2. Create release Epic
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/03-create-epic.sh

# 3. Create onboarding PR
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/04-onboard-release.sh

# 4. Create konflux PR
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/05-konflux-pr.sh
```

## Sub-Commands

### 1. CVE Discovery with SLA Monitoring (`01-discover-cves.sh`)

Finds all security issues affecting a z-stream release and monitors SLA deadlines.

**What it does:**
- Builds JQL query for security issues
- Checks affected versions (e.g., for v1.5.3, checks 1.5.0, 1.5.1, 1.5.2)
- Searches for issues with both "Global Hub" and "security" components
- Lists issues that are not Closed or Resolved
- Fetches SLA dates for each CVE from Jira
- Highlights urgency with color coding:
  - 🔴 **OVERDUE**: SLA date has passed
  - ⚠️ **CRITICAL**: ≤3 days remaining
  - ⚡ **WARNING**: ≤7 days remaining
  - ✓ **OK**: >7 days remaining

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/01-discover-cves.sh
```

**Output:**
- JQL query used for discovery
- Table with KEY, SLA DATE, URGENCY, and SUMMARY
- Summary counts (Total, Overdue, Critical, Warning)
- Issue keys for Epic linking
- Results saved to `/tmp/globalhub-{version}-cves.txt`

### 2. Epic Creation (`03-create-epic.sh`)

Creates a z-stream release Epic in Jira and links CVE issues.

**What it does:**
- Calculates release information (base version, ACM version)
- Builds Epic title, labels, and metadata
- Guides Epic creation via Claude Code or jira-administrator agent

**Usage:**
```bash
# Auto-discover CVEs
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/03-create-epic.sh

# With specific CVE list
RELEASE_VERSION=v1.5.3 CVE_LIST="ACM-28186,ACM-28100" .claude/skills/new-z-release/scripts/03-create-epic.sh
```

**Epic Details:**
- **Title:** Global Hub v1.5.3 - Z-Stream Release
- **Component:** Global Hub
- **Labels:** z-stream, release-1.5.3, global-hub
- **Work Type:** Security & Compliance
- **Priority:** High

### 3. Release Onboarding (`04-onboard-release.sh`)

Creates PR to multicluster-global-hub repository to onboard the z-stream release.

**What it does:**
- Calculates release branch (e.g., release-2.14 for v1.5.x)
- Guides PR creation with version updates, CHANGELOG, bundle regeneration

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/04-onboard-release.sh
```

**Changes Made in PR:**
- Update VERSION file
- Update CHANGELOG.md with CVE fixes
- Update operator/manager/agent versions
- Regenerate bundle with `make bundle`

### 4. Konflux Integration (`05-konflux-pr.sh`)

Creates MR to konflux-release-data repository for release configuration.

**What it does:**
- Navigates to `../../../gitlab.cee.redhat.com/konflux-release-data`
- Fetches and pulls latest changes from origin
- Updates prod and stage ReleasePlanAdmission files
- Updates `product_version` and version tags
- Creates MR using GitLab CLI (glab)

**Prerequisites:**
- GitLab CLI (glab) installed: `brew install glab`
- Authenticated to GitLab: `glab auth login`

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/05-konflux-pr.sh

# Dry-run mode
DRY_RUN=true RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/05-konflux-pr.sh
```

**Files Updated:**
- `config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/acm-multicluster-glo/acm-multicluster-glo-{MAJOR}-{MINOR}-rpa-prod.yaml`
- `config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/acm-multicluster-glo/acm-multicluster-glo-{MAJOR}-{MINOR}-rpa-stage.yaml`

## Version Format

Z-stream versions follow semantic versioning:

```
v{MAJOR}.{MINOR}.{PATCH}

Examples:
  v1.5.3  - Third z-stream for v1.5
  v1.7.1  - First z-stream for v1.7
```

**Version Mapping:**
- Global Hub v1.5.3 → ACM 2.14.z → Release Branch: release-2.14
- Global Hub v1.7.1 → ACM 2.16.z → Release Branch: release-2.16

## Workflow Example

Complete workflow for v1.5.3 release:

```bash
# Step 1: Discover CVEs with SLA monitoring
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/01-discover-cves.sh
# Result: Found 6 CVE issues
#   - 6 OVERDUE (ACM-28392, ACM-28186, ACM-27960, ACM-27836, ACM-27668, ACM-27543)
#   - SLA dates displayed with urgency highlighting
#   - Results saved to /tmp/globalhub-1.5.3-cves.txt

# Step 2: Create Epic
RELEASE_VERSION=v1.5.3 CVE_LIST="ACM-28392,ACM-28186,ACM-27960,ACM-27836,ACM-27668,ACM-27543" .claude/skills/new-z-release/scripts/03-create-epic.sh
# Use Claude Code: "Create z-stream release Epic for Global Hub v1.5.3"
# Result: Epic ACM-XXXXX created and all 6 CVE issues linked

# Step 3: Create onboarding PR
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/04-onboard-release.sh
# Use Claude Code: "Create onboarding PR for Global Hub v1.5.3"
# Result: PR #XXXX created to release-2.14 branch

# Step 4: Create konflux PR
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/05-konflux-pr.sh
# Use Claude Code: "Create konflux-release-data PR for Global Hub v1.5.3"
# Result: PR #YYY created
```

## Environment Variables

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `RELEASE_VERSION` | Z-stream version | *required* | `v1.5.3` |
| `CVE_LIST` | Comma-separated CVE issue keys | auto-discover | `ACM-28186,ACM-28100` |
| `DRY_RUN` | Preview without executing | `false` | `true` |

## Troubleshooting

### Issue: "No security issues found"

**Possible causes:**
1. Issues don't have both "Global Hub" AND "security" components
2. Issues are Closed or Resolved
3. Affected Version field not set correctly

**Solution:**
- Verify issues in Jira web UI have correct components
- Check Affected Version matches "Global Hub X.Y.Z" format
- Ensure issues are in Open/In Progress status

### Issue: "Can't find CVE in Jira"

**Possible causes:**
1. Issue has security-level restrictions
2. Issue key is incorrect
3. Authentication token doesn't have access

**Solution:**
- Verify you can see the issue in Jira web browser
- Check issue key spelling
- Contact Jira admin if restricted

## Integration with Claude Code

All scripts are designed to work with Claude Code using the jira-administrator agent:

```
Request: "Find security issues for Global Hub v1.5.3"
→ Executes CVE discovery via jira-administrator agent

Request: "Create z-stream release Epic for Global Hub v1.5.3"
→ Creates Epic and links issues via jira-administrator agent

Request: "Create onboarding PR for Global Hub v1.5.3"
→ Creates PR with version updates and changelog
```

## Best Practices

1. **Run discovery early** - Start CVE discovery at beginning of release planning
2. **Monitor SLA daily** - Check SLA reminders daily during active release cycle
3. **Test in dry-run** - Always test with `DRY_RUN=true` first
4. **Review PRs carefully** - Never auto-merge generated PRs
5. **Update Epic regularly** - Keep Epic description current with progress

## Support

- **Skill Documentation:** [SKILL.md](SKILL.md)
- **Project Issues:** https://github.com/stolostron/multicluster-global-hub/issues
- **Jira Tools:** See jira-tools plugin for Jira integration details
