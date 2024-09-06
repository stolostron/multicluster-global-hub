package models

// SecurityAlertCounts contains a summary of the security alerts from a hub.
type SecurityAlertCounts struct {
	// Low is the total number of low severity alerts.
	Low int `json:"low,omitempty"`

	// Medium is the total number of medium severity alerts.
	Medium int `json:"medium,omitempty"`

	// High is the total number of high severity alerts.
	High int `json:"high,omitempty"`

	// Critical is the total number of critical severity alerts.
	Critical int `json:"critical,omitempty"`

	// DetailURL is the URL where the user can see the details of the alerts of the hub. This
	// will typically be the URL of the violations tab of the Stackrox Central UI:
	//
	//	https://central-rhacs-operator.apps.../main/violations
	DetailURL string `json:"detail_url,omitempty"`
}
