package handler

import cloudevents "github.com/cloudevents/sdk-go/v2"

type EventHandler interface {
	ToDatabase(evt cloudevents.Event) error
}
