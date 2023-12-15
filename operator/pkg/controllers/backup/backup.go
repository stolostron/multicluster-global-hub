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

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Backup interface {
	//Add label to one object
	AddLabelToOneObj(ctx context.Context, client client.Client, namespace, name string) error

	//Add label to all objects which in one namespace
	AddLabelToAllObjs(ctx context.Context, client client.Client, namespace string) error
}
