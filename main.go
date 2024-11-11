package main

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metadata represents the JSON structure with the last_reported field.
type Metadata struct {
	ID           int                    `json:"id"`
	LastReported *timestamppb.Timestamp `json:"last_reported"` // Use the wrapper type
	ResourceType string                 `json:"resource_type"`
}

func main() {
	// Example JSON input
	jsonData := `{
		"id": 1,
		"last_reported": "2024-11-15T07:07:29.511421978Z",
		"resource_type": "k8s-cluster"
	}`

	// Parse the JSON into Metadata
	var metadata Metadata
	if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return
	}

	// Output the result
	fmt.Printf("Metadata: %+v\n", metadata)
	fmt.Println("LastReported:", metadata.LastReported)
}
