// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

// init the transport with different implementation/protocol: AMQ, strimzi operator or plain deployment
type Transport interface {
	// create the transport user(KafkaUser) if not exist for each hub clusters
	CreateUser(name string) error

	// create the transport topic(KafkaTopic) if not exist for each hub clusters
	CreateTopic(name string) error

	// get the connection credential by user
	GetConnCredential(username string) (*ConnCredential, error)
}
type ConnCredential struct {
	BootstrapServer string
	CACert          string
	ClientCert      string
	ClientKey       string
}

type ClusterTopic struct {
	SpecTopic   string
	StatusTopic string
	EventTopic  string
}
