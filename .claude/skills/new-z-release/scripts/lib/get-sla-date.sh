#!/bin/bash

# Get SLA date for a Jira issue
# Usage: get_sla_date <ISSUE-KEY>
# Returns: SLA date in format "YYYY-MM-DD" or empty string if not available

get_sla_date() {
    local issue_key="$1"

    # Try to get SLA date using jira CLI with custom output
    # First, try to get the issue details
    local issue_json

    # Use curl to get issue data from Jira API
    # Note: This works for public Jira instances or if JIRA_API_TOKEN is set
    if command -v curl &> /dev/null && command -v jq &> /dev/null; then
        local jira_url="${JIRA_URL:-https://issues.redhat.com}"
        local api_endpoint="${jira_url}/rest/api/2/issue/${issue_key}?fields=customfield_12326740,duedate"

        # Try API call (may require authentication)
        issue_json=$(curl -s -X GET \
            -H "Content-Type: application/json" \
            ${JIRA_API_TOKEN:+-H "Authorization: Bearer ${JIRA_API_TOKEN}"} \
            "$api_endpoint" 2>/dev/null)

        if [[ -n "$issue_json" ]] && echo "$issue_json" | jq -e '.fields' &>/dev/null; then
            # Try to get SLA date first (customfield_12326740)
            local sla_date=$(echo "$issue_json" | jq -r '.fields.customfield_12326740 // empty' 2>/dev/null)

            if [[ -n "$sla_date" && "$sla_date" != "null" ]]; then
                # Extract just the date part (YYYY-MM-DD) from ISO timestamp
                echo "$sla_date" | cut -d'T' -f1
                return 0
            fi

            # Fallback to due date if SLA date is not available
            local due_date=$(echo "$issue_json" | jq -r '.fields.duedate // empty' 2>/dev/null)

            if [[ -n "$due_date" && "$due_date" != "null" ]]; then
                echo "$due_date"
                return 0
            fi
        fi
    fi

    # If API approach fails, try using jira-administrator agent via Claude Code
    # This requires Claude Code to be available
    if command -v claude &> /dev/null; then
        local agent_response=$(echo "Get the SLA date (customfield_12326740) or due date for issue ${issue_key}. Return only the date in YYYY-MM-DD format." | \
            claude --agent jira-administrator 2>/dev/null | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1)

        if [[ -n "$agent_response" ]]; then
            echo "$agent_response"
            return 0
        fi
    fi

    # If all approaches fail, return empty
    echo ""
    return 1
}

# Format SLA date for display (convert YYYY-MM-DD to "Day, DD Mon YY")
format_sla_date() {
    local sla_date="$1"

    if [[ -z "$sla_date" ]]; then
        echo "N/A"
        return 1
    fi

    # Use date command to format (macOS compatible)
    if date -j -f "%Y-%m-%d" "$sla_date" "+%a, %d %b %y" 2>/dev/null; then
        return 0
    else
        # Fallback: just return the original date
        echo "$sla_date"
        return 0
    fi
}

# Calculate days remaining until SLA date
calculate_days_remaining() {
    local sla_date="$1"
    local current_date=$(date +%s)

    if [[ -z "$sla_date" ]]; then
        echo "999" # Return large number if no SLA date
        return 1
    fi

    # Parse SLA date to epoch (macOS compatible)
    local sla_epoch
    if sla_epoch=$(date -j -f "%Y-%m-%d" "$sla_date" +%s 2>/dev/null); then
        echo $(( (sla_epoch - current_date) / 86400 ))
        return 0
    fi

    echo "999"
    return 1
}
