package utils

import (
	"errors"
	"fmt"
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

func TestParseRetention(t *testing.T) {
	s := "-1y4m"
	_, e := ParseRetentionMonth(s)
	assert.EqualError(t, e, fmt.Errorf("invalid retention %s", s).Error())

	s = "2y"
	m, e := ParseRetentionMonth(s)
	assert.ErrorIs(t, e, nil)
	assert.Equal(t, 24, m)

	s = "6m"
	m, e = ParseRetentionMonth(s)
	assert.ErrorIs(t, e, nil)
	assert.Equal(t, 6, m)

	s = "3m2y"
	m, e = ParseRetentionMonth(s)
	assert.ErrorIs(t, e, nil)
	assert.Equal(t, 27, m)

	s = "-y"
	_, e = ParseRetentionMonth(s)
	assert.EqualError(t, e, fmt.Errorf("invalid retention %s", s).Error())

	s = "2+m"
	_, e = ParseRetentionMonth(s)
	assert.EqualError(t, e, fmt.Errorf("invalid retention %s", s).Error())
}
