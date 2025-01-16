package main

import (
	"context"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

func main() {
	// leafHubName := "hub1"
	// if err := managedHub(context.Background(), leafHubName); err != nil {
	// 	log.Fatalf("failed to send the k8s cluster in managed hub: %v", err)
	// }
	if err := globalHub(context.Background()); err != nil {
		log.Fatalf("failed to send the k8s cluster in global hub: %v", err)
	}
}
