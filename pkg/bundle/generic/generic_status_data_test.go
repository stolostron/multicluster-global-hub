package generic

import (
	"encoding/json"
	"fmt"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestEventData(t *testing.T) {
	e := cloudevents.NewEvent()
	data := GenericObjectData{}
	obj := &placementrulev1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-placementrule-1",
			Namespace: "default",
			UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		},
		Spec: placementrulev1.PlacementRuleSpec{
			SchedulerName: constants.GlobalHubSchedulerName,
		},
	}
	data = append(data, obj)

	byteData, err := json.Marshal(data)
	assert.Nil(t, err)

	err = e.SetData(cloudevents.ApplicationJSON, byteData)
	fmt.Println("event", e)
	assert.Nil(t, err)

	fmt.Println("original", string(e.Data()))

	parsedData := GenericObjectData{}

	err = json.Unmarshal(e.Data(), &parsedData)
	// err = e.DataAs(&parsedData)
	assert.Nil(t, err)
	fmt.Println("parsed", parsedData)
}

func TestClientObjects(t *testing.T) {
	data := []client.Object{}
	obj := &placementrulev1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-placementrule-1",
			Namespace: "default",
			UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		},
		Spec: placementrulev1.PlacementRuleSpec{
			SchedulerName: constants.GlobalHubSchedulerName,
		},
	}
	data = append(data, obj)

	byteData, err := json.Marshal(data)
	assert.Nil(t, err)
	fmt.Println("original", string(byteData))

	parsedData := []client.Object{}
	err = json.Unmarshal(byteData, &parsedData)
	assert.Nil(t, err)
	fmt.Println("parsed", parsedData)
}

func TestClientObject(t *testing.T) {
	obj := &placementrulev1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-placementrule-1",
			Namespace: "default",
			UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		},
		Spec: placementrulev1.PlacementRuleSpec{
			SchedulerName: constants.GlobalHubSchedulerName,
		},
	}

	byteObj, err := json.Marshal(obj)
	assert.Nil(t, err)
	fmt.Println("original", string(byteObj))

	var parsedObj *placementrulev1.PlacementRule
	err = json.Unmarshal(byteObj, &parsedObj)
	assert.Nil(t, err)
	fmt.Println("parsed", parsedObj)
}
