// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
)

// Message abstracts a message object to be used by different transport components.
type Message struct {
	Key     string `json:"key"`
	ID      string `json:"id"`
	MsgType string `json:"msgType"`
	Version string `json:"version"`
	Payload []byte `json:"payload"`
}

type Config struct {
	TransportType          string
	MessageCompressionType string
	CommitterInterval      time.Duration
}

const (
	// DestinationHub is the key used for destination-hub name header.
	DestinationHub = "destination-hub"
	// CompressionType is the key used for compression type header.
	CompressionType = "content-encoding"
	// Size is the key used for total bundle size header.
	Size = "size"
	// Offset is the key used for message fragment offset header.
	Offset = "offset"
	// FragmentationTimestamp is the key used for bundle fragmentation time header.
	FragmentationTimestamp = "fragmentation-timestamp"
	// Broadcast can be used as destination when a bundle should be broadcasted.
	Broadcast = ""
)

// Transport is the transport layer interface to be consumed by the spec transport bridge.
type Transport interface {
	// CustomBundleRegister registers a bundle ID to a CustomBundleRegistration. None-registered bundles are assumed to be
	// of type GenericBundle, and are handled by the GenericBundleSyncer.
	CustomBundleRegister(msgID string, customBundleRegistration *registration.CustomBundleRegistration)

	// BundleRegister function registers a msgID to the bundle updates channel.
	BundleRegister(registration *registration.BundleRegistration)

	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(msg *Message)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
