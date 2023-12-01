// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package cache

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
)

var Cache map[string]bundle.BaseAgentBundle

func RegistToCache(key string, value bundle.BaseAgentBundle) {
	if Cache == nil {
		Cache = make(map[string]bundle.BaseAgentBundle)
	}
	Cache[key] = value
}
