package config

import (
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var (
	transporter        transport.Transporter
	kafkaResourceReady = false
)

func SetTransporter(p transport.Transporter) {
	transporter = p
}

func GetTransporter() transport.Transporter {
	return transporter
}

func GetKafkaResourceReady() bool {
	return kafkaResourceReady
}

func SetKafkaResourceReady(ready bool) {
	kafkaResourceReady = ready
}

func GetKafkaStorageSize(mgh *globalhubv1alpha4.MulticlusterGlobalHub) string {
	defaultKafkaStorageSize := "10Gi"
	if mgh.Spec.DataLayer.Kafka.StorageSize != "" {
		return mgh.Spec.DataLayer.Kafka.StorageSize
	}
	return defaultKafkaStorageSize
}
