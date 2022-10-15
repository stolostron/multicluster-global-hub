// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type Producer interface {
	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(message *transport.Message)
	// Subscribe adds a callback to be delegated when a given event occurs for a message with the given ID.
	Subscribe(messageID string, callbacks map[EventType]EventCallback)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
	// SupportsDeltaBundles returns true if the transport layer supports delta bundles, otherwise false.
	SupportsDeltaBundles() bool
}

// EventType is the type of transportation-events that may occur.
type EventType string

// EventCallback is the type for subscription callbacks.
type EventCallback func()

const (
	// DeliveryAttempt event occurs when an attempted transport-delivery operation is attempted (sent to servers).
	DeliveryAttempt EventType = "attempt"
	// DeliverySuccess event occurs when an attempted transport-delivery operation is successful (ack from servers).
	DeliverySuccess EventType = "success"
	// DeliveryFailure event occurs when an attempted transport-delivery operation fails.
	DeliveryFailure EventType = "failure"
)

// InvokeCallback invokes relevant callback in the given events subscription map.
func InvokeCallback(eventSubscriptionMap map[string]map[EventType]EventCallback,
	messageID string, eventType EventType,
) {
	callbacks, found := eventSubscriptionMap[messageID]
	if !found {
		return
	}

	if callback, found := callbacks[eventType]; found {
		callback()
	}
}
