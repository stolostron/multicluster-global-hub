package security

import (
	"fmt"
	"strconv"

	wiremodels "github.com/stolostron/multicluster-global-hub/pkg/wire/models"
)

const (
	StackRoxResponseLowSeverity      = "LOW_SEVERITY"
	StackRoxResponseMediumSeverity   = "MEDIUM_SEVERITY"
	StackRoxResponseHighSeverity     = "HIGH_SEVERITY"
	StackRoxResponseCriticalSeverity = "CRITICAL_SEVERITY"
	stackRoxAlertsDetailsPath        = "/main/violations"
	stackRoxAlertsSummaryCountsPath  = "/v1/alerts/summary/counts"
)

type Count struct {
	Severity string `json:"severity"`
	Count    string `json:"count"`
}

type Group struct {
	Group  string  `json:"group"`
	Counts []Count `json:"counts"`
}

type AlertsSummeryCountsResponse struct {
	Groups []Group `json:"groups"`
}

var AlertsSummeryCountsRequest = stackRoxRequest{
	Method:      "GET",
	Path:        stackRoxAlertsSummaryCountsPath,
	Body:        "",
	CacheStruct: &AlertsSummeryCountsResponse{},
	GenerateFromCache: func(values ...any) (any, error) {
		if len(values) != 2 {
			return nil, fmt.Errorf("alert summery count cache struct or ACS base URL were not provided")
		}

		alertCountSummeryResponse, ok := values[0].(*AlertsSummeryCountsResponse)
		if !ok {
			return nil, fmt.Errorf("alert summery count cache struct is not of the right type")
		}

		if len(alertCountSummeryResponse.Groups) != 1 {
			return nil, fmt.Errorf("no result groups returned from ACS")
		}

		acsCentralExternalHostPort, ok := values[1].(string)
		if !ok {
			return nil, fmt.Errorf("ACS external URL is not valid")
		}
		alertCount := wiremodels.SecurityAlertCounts{
			DetailURL: fmt.Sprintf("%s%s", acsCentralExternalHostPort, stackRoxAlertsDetailsPath),
		}

		for _, count := range alertCountSummeryResponse.Groups[0].Counts {
			countInt, err := convertSringToInt(count.Count)
			if err != nil {
				return nil, fmt.Errorf("failed to convert %s to integer: %v", count.Count, err)
			}

			switch count.Severity {
			case StackRoxResponseLowSeverity:
				alertCount.Low = *countInt

			case StackRoxResponseMediumSeverity:

				alertCount.Medium = *countInt

			case StackRoxResponseHighSeverity:
				alertCount.High = *countInt

			case StackRoxResponseCriticalSeverity:
				alertCount.Critical = *countInt
			}
		}

		return alertCount, nil
	},
}

func convertSringToInt(s string) (*int, error) {
	num, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &num, nil
}
