package protocol

import (
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	DEFAULT_SPEC_TOPIC          = "gh-spec"
	DEFAULT_STATUS_TOPIC        = "gh-status.*"
	DEFAULT_SHARED_STATUS_TOPIC = "gh-status"
)

// transport protocol
// indicate which kind of transport protocol, only support
type TransportType int

const (
	StrimziTransport TransportType = iota
	// the kafka cluster is created by customer, and the transport secret will be shared between clusters
	BYOTransport
)

// Transporter used to initialize the infras, it has different implementation/protocol:
// byo_secret, strimzi operator or plain deployment
type Transporter interface {
	// Validate the configuration from the mg CR
	Validate() error

	// Initialize the resources for the kafka cluster
	Initialize() error

	// CreateUser creates/updates a kafka user for the cluster, the kafka user name is also the CN of cert
	EnsureUser(clusterName string) (string, error)

	// CreateTopic creates/updates a kafka topic
	EnsureTopic(clusterName string) (*transport.ClusterTopic, error)

	// Cleanup will delete the user or topic for the cluster
	Prune(clusterName string) error

	// get the connection credential by clusterName, ready indicates whether the connection is ready
	GetConnCredential(clusterName string) (bool, *transport.KafkaConfig, error)
}
