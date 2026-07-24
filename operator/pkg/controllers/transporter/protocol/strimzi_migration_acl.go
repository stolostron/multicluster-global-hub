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

func migrationWriteACLKey(topic string) string {
	return utils.GenerateACLKey(utils.WriteTopicACL(topic))
}

func (k *strimziTransporter) hasMigrationWriteACL(acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) bool {
	key := migrationWriteACLKey(config.GetMigrationTopic())
	for _, acl := range acls {
		if utils.GenerateACLKey(acl) == key {
			return true
		}
	}
	return false
}

// SyncMigrationWriteACL grants or revokes Write on the migration topic for a source hub.
func (k *strimziTransporter) SyncMigrationWriteACL(fromHub string, grant bool) error {
	if fromHub == "" {
		return nil
	}

	userName := config.GetKafkaUserName(fromHub)
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      userName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		if !grant {
			return nil
		}
		return fmt.Errorf("kafka user %s not found for migration write ACL", userName)
	}
	if err != nil {
		return err
	}

	migrationTopic := config.GetMigrationTopic()
	desiredWriteACL := utils.WriteTopicACL(migrationTopic)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestKafkaUser := &kafkav1beta2.KafkaUser{}
		if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      userName,
			Namespace: k.kafkaClusterNamespace,
		}, latestKafkaUser); err != nil {
			return err
		}

		currentACLs := currentKafkaUserACLs(latestKafkaUser)
		if k.hasMigrationWriteACL(currentACLs) == grant {
			return nil
		}

		updatedACLs := make([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, 0, len(currentACLs))
		for _, acl := range currentACLs {
			if utils.GenerateACLKey(acl) == migrationWriteACLKey(migrationTopic) {
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
		return k.manager.GetClient().Update(k.ctx, latestKafkaUser)
	})
}

func currentKafkaUserACLs(kafkaUser *kafkav1beta2.KafkaUser) []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	if kafkaUser == nil || kafkaUser.Spec == nil || kafkaUser.Spec.Authorization == nil {
		return nil
	}
	return kafkaUser.Spec.Authorization.Acls
}
