#!/bin/bash

# Date utility functions for z-stream release workflow

# Calculate days between two dates
# Usage: days_between "2026-01-19" "2026-01-26"
days_between() {
    local date1="$1"
    local date2="$2"

    # Convert dates to seconds since epoch
    local epoch1 epoch2

    if [[ "$(uname)" == "Darwin" ]]; then
        # macOS
        epoch1=$(date -j -f "%Y-%m-%d" "$date1" "+%s" 2>/dev/null)
        epoch2=$(date -j -f "%Y-%m-%d" "$date2" "+%s" 2>/dev/null)
    else
        # Linux
        epoch1=$(date -d "$date1" "+%s" 2>/dev/null)
        epoch2=$(date -d "$date2" "+%s" 2>/dev/null)
    fi

    if [[ -z "$epoch1" || -z "$epoch2" ]]; then
        echo "0"
        return 1
    fi

    # Calculate difference in days
    local diff_seconds=$((epoch2 - epoch1))
    local diff_days=$((diff_seconds / 86400))

    echo "$diff_days"
}

# Get current date in YYYY-MM-DD format
current_date() {
    date "+%Y-%m-%d"
}

# Get current datetime in ISO 8601 format
current_datetime() {
    date "+%Y-%m-%dT%H:%M:%S%z"
}

# Add days to a date
# Usage: add_days "2026-01-19" 7
add_days() {
    local date="$1"
    local days="$2"

    if [[ "$(uname)" == "Darwin" ]]; then
        # macOS
        date -j -v "+${days}d" -f "%Y-%m-%d" "$date" "+%Y-%m-%d" 2>/dev/null
    else
        # Linux
        date -d "$date + $days days" "+%Y-%m-%d" 2>/dev/null
    fi
}

# Parse ISO 8601 datetime to YYYY-MM-DD
# Usage: parse_iso_date "2026-01-19T10:30:00-0500"
parse_iso_date() {
    local datetime="$1"

    # Extract date part (YYYY-MM-DD)
    echo "$datetime" | cut -d'T' -f1
}

# Calculate SLA urgency level based on days remaining
# Usage: get_sla_urgency 5
get_sla_urgency() {
    local days_remaining="$1"

    if [[ $days_remaining -le 0 ]]; then
        echo "OVERDUE"
    elif [[ $days_remaining -le 3 ]]; then
        echo "CRITICAL"
    elif [[ $days_remaining -le 7 ]]; then
        echo "HIGH"
    elif [[ $days_remaining -le 14 ]]; then
        echo "MEDIUM"
    else
        echo "LOW"
    fi
}
