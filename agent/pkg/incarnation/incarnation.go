// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package incarnation

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	BASE10      = 10
	UINT64_SIZE = 64
)

// Incarnation is a part of the version of all the messages this process will transport.
// The motivation behind this logic is allowing the message receivers/consumers to infer that messages transmitted
// from this instance are more recent than all other existing ones, regardless of their instance-specific generations.
func GetIncarnation(mgr ctrl.Manager) (uint64, error) {
	k8sClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return 0, fmt.Errorf("failed to start k8s client - %w", err)
	}

	ctx := context.Background()
	configMap := &corev1.ConfigMap{}

	// try to get ConfigMap
	objKey := client.ObjectKey{
		Namespace: constants.GHSystemNamespace,
		Name:      constants.GHAgentIncarnationCMName,
	}
	if err := k8sClient.Get(ctx, objKey, configMap); err != nil {
		if !apiErrors.IsNotFound(err) {
			return 0, fmt.Errorf("failed to get incarnation config-map - %w", err)
		}

		// incarnation ConfigMap does not exist, create it with incarnation = 0
		configMap = CreateIncarnationConfigMap(0)
		if err := k8sClient.Create(ctx, configMap); err != nil {
			return 0, fmt.Errorf("failed to create incarnation config-map obj - %w", err)
		}

		return 0, nil
	}

	// incarnation configMap exists, get incarnation, increment it and update object
	incarnationString, exists := configMap.Data[constants.GHAgentIncarnationCMKey]
	if !exists {
		return 0, fmt.Errorf("configmap %s does not contain (%s)",
			constants.GHAgentIncarnationCMKey, constants.GHAgentIncarnationCMKey)
	}

	lastIncarnation, err := strconv.ParseUint(incarnationString, BASE10, UINT64_SIZE)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value of key %s in configmap %s - %w", constants.GHAgentIncarnationCMKey,
			constants.GHAgentIncarnationCMKey, err)
	}

	newConfigMap := CreateIncarnationConfigMap(lastIncarnation + 1)
	if err := k8sClient.Patch(ctx, newConfigMap, client.MergeFrom(configMap)); err != nil {
		return 0, fmt.Errorf("failed to update incarnation version - %w", err)
	}

	return lastIncarnation + 1, nil
}

func CreateIncarnationConfigMap(incarnation uint64) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.GHSystemNamespace,
			Name:      constants.GHAgentIncarnationCMName,
		},
		Data: map[string]string{constants.GHAgentIncarnationCMKey: strconv.FormatUint(incarnation, BASE10)},
	}
}
