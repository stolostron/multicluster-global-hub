package helpers

import (
	"fmt"
	"os"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestLoadCACertcoConfigMap(t *testing.T) {
	cases := []struct {
		desc           string
		caPath         string
		caExists       bool
		kafkaConfigMap *kafka.ConfigMap
		expectedErr    error
	}{
		{
			"ca path doesn't exists",
			"/do/not/exist",
			false,
			&kafka.ConfigMap{},
			fmt.Errorf("failed to read ca certificate"),
		},
		{
			"ca exists but is not valid",
			"/tmp/testinvalidca",
			true,
			&kafka.ConfigMap{},
			fmt.Errorf("failed to read ca certificate"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.caExists {
				if err := os.WriteFile(tc.caPath, []byte("foo\n"), 0o644); err != nil {
					t.Errorf("failed to write test ca to file - %v", err)
				}
			}
			err := LoadCACertToConfigMap(tc.caPath, tc.kafkaConfigMap)
			t.Logf("error: %v", err)
			if (err != nil && tc.expectedErr == nil) || (err == nil && tc.expectedErr != nil) {
				t.Errorf("%s:\nexpected err: %v\ngot err: %v\n",
					tc.desc, tc.expectedErr, err)
			}
		})
	}
}
