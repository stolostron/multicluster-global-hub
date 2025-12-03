package config

import (
	"fmt"
	"testing"
)

// TestIsValidKafkaTopicName tests the isValidKafkaTopicName function.
func TestIsValidKafkaTopicName(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid topic with letters and numbers", "validTopic1", true},
		{"Valid topic with underscore", "valid_topic-2", true},
		{"Valid topic with dot", "valid.topic.3", true},
		{"Invalid topic with space", "invalid topic", false},
		{"Invalid topic with invalid character", "invalidTopic$", false},
		{"Empty topic name", "", false},
		{"Invalid name .", ".", false},
		{"Invalid name ..", "..", false},
		{"Valid topic with asterisk", "validTopic*", true},
		{"Invalid topic with asterisk not at the end", "invalidTopic*extra", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := IsValidKafkaTopicName(testCase.input)
			if result != testCase.expected {
				fmt.Println("len", len(testCase.input))
				t.Errorf("isValidKafkaTopicName(%q) = %v; expected %v", testCase.input, result, testCase.expected)
			}
		})
	}
}
