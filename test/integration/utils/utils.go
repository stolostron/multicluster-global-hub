/*
Copyright 2023

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

package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

func DeleteMgh(ctx context.Context, runtimeClient client.Client, mgh *v1alpha4.MulticlusterGlobalHub) error {
	curmgh := &v1alpha4.MulticlusterGlobalHub{}

	err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), curmgh)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(curmgh.Finalizers) != 0 {
		deletemgh := curmgh.DeepCopy()
		deletemgh.Finalizers = []string{}
		err = runtimeClient.Update(ctx, deletemgh)
		if err != nil {
			return err
		}
	}

	err = runtimeClient.Delete(ctx, mgh)
	if err != nil {
		return err
	}

	if err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh); errors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf("mgh instance should be deleted: %s/%s", mgh.Namespace, mgh.Name)
}
