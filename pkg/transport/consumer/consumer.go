// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	bundle "github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
)

// Transport is an interface for transport layer.
type Consumer interface {
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
	// CustomBundleRegister registers a bundle ID to a CustomBundleRegistration. None-registered bundles are assumed to be
	// of type GenericBundle, and are handled by the GenericBundleSyncer.
	CustomBundleRegister(msgID string, customBundleRegistration *registration.CustomBundleRegistration)

	// BundleRegister function registers a msgID to the bundle updates channel.
	BundleRegister(registration *registration.BundleRegistration)

	// provide the generic bundle for message producer
	GetGenericBundleChan() chan *bundle.GenericBundle
}
