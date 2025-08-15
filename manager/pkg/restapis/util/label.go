// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"fmt"
	"strings"
)

const (
	invalidLabelSelectorFormatMsg = "invalid equality based label selector: %s"
)

func ParseLabelSelector(labelSelector string) (string, error) {
	selectorInSql := ""
	selectors := strings.Split(labelSelector, ",")
	for _, selector := range selectors {
		switch {
		case strings.Contains(selector, "!="):
			keyValPair := strings.Split(selector, "!=")
			if len(keyValPair) != 2 {
				return "", fmt.Errorf(invalidLabelSelectorFormatMsg, selector)
			}

			key, val := strings.TrimSpace(keyValPair[0]), strings.TrimSpace(keyValPair[1])
			selectorInSql += fmt.Sprintf(" AND NOT (payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}')", key, val)
		case strings.Contains(selector, "==") || strings.Contains(selector, "="):
			var keyValPair []string
			if strings.Contains(selector, "==") {
				keyValPair = strings.Split(selector, "=")
			} else if strings.Contains(selector, "=") {
				keyValPair = strings.Split(selector, "=")
			}

			if len(keyValPair) != 2 {
				return "", fmt.Errorf(invalidLabelSelectorFormatMsg, selector)
			}

			key, val := strings.TrimSpace(keyValPair[0]), strings.TrimSpace(keyValPair[1])
			selectorInSql += fmt.Sprintf(" AND payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}'", key, val)
		// case strings.Contains(selector, "notin"):
		// 	keyValSetPair := strings.Split(selector, "notin")
		// 	if len(keyValSetPair) != 2 {
		// 		return "", fmt.Errorf(invalidLabelSelectorFormatMsg, selector)
		// 	}

		// 	key, valSet := strings.TrimSpace(keyValSetPair[0]), strings.TrimSpace(keyValSetPair[1])
		// 	valSet = strings.TrimPrefix(valSet, "(")
		// 	valSet = strings.TrimSuffix(valSet, ")")
		// 	for i, valStr := range strings.Split(valSet, ",") {
		// 		val := strings.TrimSpace(valStr)
		// 		if i == 0 {
		// 			selectorInSql += fmt.Sprintf(" AND (NOT (payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}')", key, val)
		// 		} else {
		// 			selectorInSql += fmt.Sprintf(" OR NOT (payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}')", key, val)
		// 		}
		// 	}
		// 	selectorInSql += ")"
		// case strings.Contains(selector, "in"):
		// 	keyValSetPair := strings.Split(selector, "in")
		// 	if len(keyValSetPair) != 2 {
		// 		return "", fmt.Errorf(invalidLabelSelectorFormatMsg, selector)
		// 	}

		// 	key, valSet := strings.TrimSpace(keyValSetPair[0]), strings.TrimSpace(keyValSetPair[1])
		// 	valSet = strings.TrimPrefix(valSet, "(")
		// 	valSet = strings.TrimSuffix(valSet, ")")
		// 	for i, valStr := range strings.Split(valSet, ",") {
		// 		val := strings.TrimSpace(valStr)
		// 		if i == 0 {
		// 			selectorInSql += fmt.Sprintf(" AND (payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}'", key, val)
		// 		} else {
		// 			selectorInSql += fmt.Sprintf(" OR payload -> 'metadata' -> 'labels' @> '{\"%s\": \"%s\"}'", key, val)
		// 		}
		// 	}
		// 	selectorInSql += ")"
		case strings.HasPrefix(selector, "!"):
			key := strings.TrimSpace(strings.TrimPrefix(selector, "!"))
			selectorInSql += fmt.Sprintf(" AND NOT (payload -> 'metadata' -> 'labels' ? '%s')", key)
		default:
			key := strings.TrimSpace(selector)
			selectorInSql += fmt.Sprintf(" AND payload -> 'metadata' -> 'labels' ? '%s'", key)
		}
	}

	return selectorInSql, nil
}
