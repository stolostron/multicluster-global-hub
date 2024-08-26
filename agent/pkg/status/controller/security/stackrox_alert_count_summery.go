package security

import (
	"fmt"
	"net/url"
	"strconv"
)

const (
	ACSResponseLowSeverity      string = "LOW_SEVERITY"
	ACSResponseMediumSeverity   string = "MEDIUM_SEVERITY"
	ACSResponseHighSeverity     string = "HIGH_SEVERITY"
	ACSResponseCriticalSeverity string = "CRITICAL_SEVERITY"
	alertsDetailsPath           string = "main/violations"
	alertSummeryCountPath       string = "/v1/alerts/summary/counts"
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

type alertCountMessage struct {
	Low       int    `json:"low"`
	Medium    int    `json:"medium"`
	High      int    `json:"high"`
	Critical  int    `json:"critical"`
	DetailURL string `json:"detail_url"`
}

var AlertsSummeryCountsRequest = stackroxCentralRequest{
	Method:      "GET",
	Path:        alertSummeryCountPath,
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
		acsViolationDetailsURL := url.URL{Scheme: "https", Host: acsCentralExternalHostPort, Path: alertsDetailsPath}
		alertCount := alertCountMessage{DetailURL: acsViolationDetailsURL.String()}

		for _, count := range alertCountSummeryResponse.Groups[0].Counts {
			countInt, err := convertSringToInt(count.Count)
			if err != nil {
				return nil, fmt.Errorf("failed to convert %s to integer: %v", count.Count, err)
			}

			switch count.Severity {
			case ACSResponseLowSeverity:
				alertCount.Low = *countInt

			case ACSResponseMediumSeverity:

				alertCount.Medium = *countInt

			case ACSResponseHighSeverity:
				alertCount.High = *countInt

			case ACSResponseCriticalSeverity:
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
