// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGenericProducer(t *testing.T) {
	p := &GenericProducer{}
	tranConfig := &transport.TransportConfig{
		TransportType: string(transport.Rest),
		KafkaCredential: &transport.KafkaConnCredential{
			SpecTopic:   "gh-spec",
			StatusTopic: "gh-status",
		},
	}
	err := p.initClient(tranConfig)
	require.Equal(t, "the restful credentail must not be nil", err.Error())
}
