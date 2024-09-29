package requester

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func TestReqeust(t *testing.T) {
	inventoryClient := &InventoryClient{}

	evt := cloudevents.NewEvent()
	evt.SetType(string(enum.ManagedClusterInfoType))
	err := inventoryClient.Request(context.Background(), evt)
	assert.Nil(t, err)
}
