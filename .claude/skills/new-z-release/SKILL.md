---
name: new-z-release
description: "Automate z-stream release workflow for Global Hub including CVE tracking with SLA monitoring, Epic creation, and PR creation. Each sub-command can be run independently: discover CVEs with SLA dates, create Epic, onboard release, and create konflux PR."
---

# Global Hub Z-Stream Release Workflow

Automates z-stream release management for Multicluster Global Hub with four independent sub-commands.

## Sub-Commands Overview

### 1. CVE Discovery with SLA Monitoring (`01-discover-cves.sh`)
Finds security issues for a z-stream release and monitors SLA deadlines.

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/01-discover-cves.sh
```

**What it does:**
- Builds JQL query: `component = "Global Hub" AND component = security AND affectedVersion in (...)`
- For v1.5.3, checks versions: 1.5.0, 1.5.1, 1.5.2
- Executes jira CLI directly to fetch issues
- Fetches SLA date for each CVE from Jira
- Highlights urgency with color coding:
  - 🔴 OVERDUE: SLA date has passed
  - ⚠️ CRITICAL: ≤3 days remaining
  - ⚡ WARNING: ≤7 days remaining
  - ✓ OK: >7 days remaining
- Saves results to `/tmp/globalhub-{version}-cves.txt`

**Output:**
- Table showing KEY, SLA DATE, URGENCY, and SUMMARY
- Summary counts (Total, Overdue, Critical, Warning)
- Issue keys ready for Epic linking

### 2. Epic Creation (`03-create-epic.sh`)
Creates z-stream release Epic and links CVE issues.

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/03-create-epic.sh
RELEASE_VERSION=v1.5.3 CVE_LIST="ACM-28186" .claude/skills/new-z-release/scripts/03-create-epic.sh
```

**What it does:**
- Calculates Epic metadata (title, labels, versions)
- Epic title format: "Global Hub v1.5.3 - Z-Stream Release"
- Labels: z-stream, release-1.5.3, global-hub
- Work Type: Security & Compliance

**When user requests Epic creation:**
1. Run the script to get Epic details
2. Use jira-administrator agent to:
   - Create Epic with proper metadata
   - Link CVE issues (from CVE_LIST or auto-discovered)
   - Set fix version to RELEASE_VERSION

### 3. Release Onboarding (`04-onboard-release.sh`)
Creates PR to multicluster-global-hub repository.

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/04-onboard-release.sh
```

**What it does:**
- Calculates release branch (v1.5.x → release-2.14)
- Guides PR creation for version updates

**When user requests onboarding PR:**
1. Run the script to get release information
2. Create PR with:
   - Update VERSION file to v1.5.3
   - Update CHANGELOG.md with CVE fixes
   - Update operator/manager/agent versions
   - Run `make bundle` to regenerate bundle
   - Commit with sign-off and push

### 4. Konflux Integration (`05-konflux-pr.sh`)
Creates PR to konflux-release-data repository.

**Usage:**
```bash
RELEASE_VERSION=v1.5.3 .claude/skills/new-z-release/scripts/05-konflux-pr.sh
```

**What it does:**
- Guides PR creation to konflux-release-data
- Updates release configurations

**When user requests konflux PR:**
1. Run the script to get release information
2. Create PR with release configuration updates

## Execution Instructions for Claude

### For CVE Discovery Requests

When user says: "Find security issues for Global Hub v1.5.3" or "Discover CVEs for v1.5.3"

**Steps:**
1. Run `01-discover-cves.sh` to get JQL query
2. Use Task tool with jira-administrator agent:
   ```
   Task(
     subagent_type="jira-tools:jira-administrator",
     prompt="Find security issues for Global Hub v1.5.3 using this JQL: [PASTE JQL FROM SCRIPT]",
     description="Discover CVE issues for v1.5.3"
   )
   ```
3. Present results with issue keys, summaries, SLA dates, priorities

### For SLA Reminder Requests

When user says: "Check CVE SLA reminders" or "Monitor SLA deadlines"

**Steps:**
1. Run `02-check-sla-reminders.sh` to get JQL query
2. Use Task tool with jira-administrator agent
3. Calculate days remaining for each CVE
4. Format reminder report with urgency levels

### For Epic Creation Requests

When user says: "Create z-stream release Epic for v1.5.3"

**Steps:**
1. Run `03-create-epic.sh` to get Epic details
2. Use Task tool with jira-administrator agent to:
   - Create Epic
   - Link CVE issues
3. Return Epic key and URL

### For Onboarding PR Requests

When user says: "Create onboarding PR for v1.5.3"

**Steps:**
1. Run `04-onboard-release.sh` to get release branch info
2. Clone repo and checkout release branch
3. Create PR branch
4. Update VERSION, CHANGELOG, version metadata
5. Run `make bundle`
6. Commit with sign-off
7. Push and create PR using `gh pr create`

### For Konflux PR Requests

When user says: "Create konflux PR for v1.5.3"

**Steps:**
1. Run `05-konflux-pr.sh` to get release info
2. Create PR to konflux-release-data repository
3. Update release configurations

## Version Calculations

**For v1.5.3:**
- Major: 1
- Minor: 5
- Patch: 3
- Base Version: v1.5.0
- ACM Version: 2.14 (minor + 9)
- Release Branch: release-2.14
- Affected Versions: Global Hub 1.5.0, 1.5.1, 1.5.2

**For v1.7.1:**
- Major: 1
- Minor: 7
- Patch: 1
- Base Version: v1.7.0
- ACM Version: 2.16 (minor + 9)
- Release Branch: release-2.16
- Affected Versions: Global Hub 1.7.0

## JQL Query Pattern

The skill uses this pattern for finding z-stream security issues:

```jql
project = ACM
AND component = "Global Hub"
AND component = security
AND status not in (Closed, Resolved)
AND "Affected Version" in ("Global Hub X.Y.0", "Global Hub X.Y.1", ...)
ORDER BY priority DESC, created DESC
```

**Important:** This requires BOTH "Global Hub" AND "security" components to be set on issues.

## Integration with Jira-Administrator Agent

All Jira operations should be delegated to the jira-administrator agent:

- **CVE Discovery:** Use agent to execute JQL query
- **SLA Monitoring:** Use agent to find CVEs with approaching SLA
- **Epic Creation:** Use agent to create Epic and link issues

**Never:**
- Use Bash to run `jira` CLI commands directly
- Try to access Jira API without the agent
- Guess or fabricate Jira issue data

**Always:**
- Run the script first to get correct parameters
- Delegate to jira-administrator agent
- Present results from agent response

## Error Handling

If script fails or agent returns no results:

1. Check RELEASE_VERSION format (must be vX.Y.Z where Z ≥ 1)
2. Verify Jira issues have correct components and Affected Version
3. Check if issues are embargoed (security-level restricted)
4. Suggest manual verification in Jira web UI

## Best Practices

1. **Always run scripts first** - They calculate correct parameters
2. **Use jira-administrator agent** - Don't use CLI directly
3. **Verify version format** - Must be v1.5.3 style (not 1.5.3)
4. **Check prerequisites** - Ensure gh CLI authenticated for PR creation
5. **Review before executing** - Show user what will be done

## Example Complete Workflow

User: "I need to plan a new z-stream release for v1.5.3"

**Response:**
1. Run `01-discover-cves.sh` to find CVEs
2. Use jira-administrator to execute discovery
3. Present found CVEs with SLA info
4. Run `02-check-sla-reminders.sh` for urgency assessment
5. Run `03-create-epic.sh` to show Epic details
6. Ask user if they want to proceed with Epic creation
7. After Epic created, ask if they want onboarding PRs
8. Run `04-onboard-release.sh` and `05-konflux-pr.sh` as needed

## Notes

- All scripts are in `.claude/skills/new-z-release/scripts/`
- Scripts are standalone and can run independently
- Each script provides clear output for next steps
- Scripts guide Claude on how to use jira-administrator agent
- Always commit with `--signoff` flag for PR creation
