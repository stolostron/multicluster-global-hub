// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"fmt"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func hubHASpecWriteACLKey(specTopic string) string {
	return utils.GenerateACLKey(utils.WriteTopicACL(specTopic))
}

func (k *strimziTransporter) hasHubHASpecWriteACL(
	acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
	specTopic string,
) bool {
	key := hubHASpecWriteACLKey(specTopic)
	for _, acl := range acls {
		if utils.GenerateACLKey(acl) == key {
			return true
		}
	}
	return false
}

// SyncHubHASpecWriteACL grants or revokes Write-only on gh-spec for Hub HA active hubs.
func (k *strimziTransporter) SyncHubHASpecWriteACL(activeHub string, grant bool) error {
	if activeHub == "" {
		return nil
	}

	userName := config.GetKafkaUserName(activeHub)
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      userName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		if !grant {
			return nil
		}
		return fmt.Errorf("kafka user %s not found for Hub HA spec write ACL", userName)
	}
	if err != nil {
		return fmt.Errorf("get kafka user %s for Hub HA spec write ACL: %w", userName, err)
	}

	specTopic := config.GetSpecTopic()
	desiredWriteACL := utils.WriteTopicACL(specTopic)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestKafkaUser := &kafkav1beta2.KafkaUser{}
		if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      userName,
			Namespace: k.kafkaClusterNamespace,
		}, latestKafkaUser); err != nil {
			return fmt.Errorf("get kafka user %s for Hub HA spec write ACL update: %w", userName, err)
		}

		currentACLs := currentKafkaUserACLs(latestKafkaUser)
		if k.hasHubHASpecWriteACL(currentACLs, specTopic) == grant {
			return nil
		}

		updatedACLs := make([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, 0, len(currentACLs))
		for _, acl := range currentACLs {
			if utils.GenerateACLKey(acl) == hubHASpecWriteACLKey(specTopic) {
				continue
			}
			updatedACLs = append(updatedACLs, acl)
		}
		if grant {
			updatedACLs = append(updatedACLs, desiredWriteACL)
		}

		if len(updatedACLs) == 0 {
			latestKafkaUser.Spec.Authorization = nil
		} else {
			if latestKafkaUser.Spec.Authorization == nil {
				latestKafkaUser.Spec.Authorization = &kafkav1beta2.KafkaUserSpecAuthorization{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
				}
			}
			if latestKafkaUser.Spec.Authorization.Type == "" {
				latestKafkaUser.Spec.Authorization.Type = kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple
			}
			latestKafkaUser.Spec.Authorization.Acls = updatedACLs
		}
		if err := k.manager.GetClient().Update(k.ctx, latestKafkaUser); err != nil {
			return fmt.Errorf("update kafka user %s Hub HA spec write ACL: %w", userName, err)
		}
		return nil
	})
}
