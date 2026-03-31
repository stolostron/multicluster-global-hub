package spec

import (
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestIsEventExpired(t *testing.T) {
	tests := []struct {
		name    string
		evt     func() *cloudevents.Event
		expired bool
	}{
		{
			name: "expired event",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				e.SetExtension(constants.CloudEventExtensionKeyExpireTime,
					time.Now().Add(-1*time.Minute).Format(time.RFC3339))
				return &e
			},
			expired: true,
		},
		{
			name: "valid event",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				e.SetExtension(constants.CloudEventExtensionKeyExpireTime,
					time.Now().Add(10*time.Minute).Format(time.RFC3339))
				return &e
			},
			expired: false,
		},
		{
			name: "no expiry extension",
			evt: func() *cloudevents.Event {
				e := cloudevents.NewEvent()
				return &e
			},
			expired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expired, _ := isEventExpired(tt.evt())
			assert.Equal(t, tt.expired, expired)
		})
	}
}
