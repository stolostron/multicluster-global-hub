package utils

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDuration(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected time.Duration
		err      error
	}{
		{
			name:     "verify hour",
			input:    "2h",
			expected: 0,
			err:      errors.New("time: unknown unit " + quote("h") + " in duration " + quote("2h")),
		},
		{
			name:     "verify day",
			input:    "-3d",
			expected: 0,
			err:      errors.New("time: unknown unit " + quote("d") + " in duration " + quote("-3d")),
		},
		{
			name:     "verify month",
			input:    "4m",
			expected: 4 * 30 * 24 * time.Hour,
			err:      nil,
		},
		{
			name:     "verify year",
			input:    "5y",
			expected: 5 * 365 * 24 * time.Hour,
			err:      nil,
		},
		{
			name:     "hybrid duration",
			input:    "4m5y",
			expected: 4*30*24*time.Hour + 5*365*24*time.Hour,
			err:      nil,
		},
		{
			name:     "invalid duration with non-numeric character",
			input:    "-..4m5y",
			expected: 0,
			err:      errors.New(InvalidDurationMessage + quote("-..4m5y")),
		},
		{
			name:     "invalid duration with non-ASCII character",
			input:    "\u263A",
			expected: 0,
			err:      errors.New(InvalidDurationMessage + quote("\u263A")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			duration, err := ParseDuration(tc.input)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, tc.expected, duration)
		})
	}
}
