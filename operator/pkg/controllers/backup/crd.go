/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backup

import (
	"context"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var crdList = sets.NewString(
	"kafkas.kafka.strimzi.io",
	"kafkatopics.kafka.strimzi.io",
	"kafkausers.kafka.strimzi.io",
)

type crdBackup struct {
	backupType string
	labelKey   string
	labelValue string
	backupSets sets.String
}

func NewCrdBackup() *crdBackup {
	return &crdBackup{
		backupType: crdType,
		backupSets: crdList,
		labelKey:   constants.BackupKey,
		labelValue: constants.BackupGlobalHubValue,
	}
}

func (r *crdBackup) AddLabelToOneObj(ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	obj := &apiextensionsv1.CustomResourceDefinition{}
	return addLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
}

func (r *crdBackup) AddLabelToAllObjs(ctx context.Context, client client.Client, namespace string) error {
	for name := range crdList {
		obj := &apiextensionsv1.CustomResourceDefinition{}
		err := addLabel(ctx, client, obj, namespace, name, r.labelKey, r.labelValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *crdBackup) DeleteLabelOfAllObjs(ctx context.Context, client client.Client, namespace string) error {
	for name := range crdList {
		obj := &apiextensionsv1.CustomResourceDefinition{}
		err := deleteLabel(ctx, client, obj, namespace, name, r.labelKey)
		if err != nil {
			return err
		}
	}
	return nil
}
