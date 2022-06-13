package transport

// Broadcast can be used as destination when a bundle should be broadcasted.
const Broadcast = ""

// Transport is the transport layer interface to be consumed by the spec transport bridge.
type Transport interface {
	// SendAsync sends a message to the transport component asynchronously.
	//
	// destinationHubName specifies a specific destination for distribution or specifies broadcasting if empty.
	SendAsync(destinationHubName string, id string, msgType string, version string, payload []byte)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
