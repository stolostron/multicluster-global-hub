/*
Copyright 2024.

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

package operator

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

// go test ./test/integration/operator -ginkgo.focus "OLM detection" -v
var _ = Describe("OLM detection", Ordered, func() {
	It("should succeed with GetAPIReader before manager starts", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: config.GetRuntimeScheme(),
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).ToNot(HaveOccurred(), "failed to create manager")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		crd := &apiextensionsv1.CustomResourceDefinition{}
		err = mgr.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: "clusterextensions.olm.operatorframework.io",
		}, crd)
		Expect(err).To(HaveOccurred(), "expected API reader Get to fail for non-existent CRD")
		Expect(strings.Contains(err.Error(), "not found")).To(BeTrue(),
			"expected NotFound error from API reader, got: %v", err)
	})

	It("should fail with GetClient before manager starts", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: config.GetRuntimeScheme(),
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).ToNot(HaveOccurred(), "failed to create manager")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		crd := &apiextensionsv1.CustomResourceDefinition{}
		err = mgr.GetClient().Get(ctx, types.NamespacedName{
			Name: "clusterextensions.olm.operatorframework.io",
		}, crd)
		Expect(err).To(HaveOccurred(), "expected cached client Get to fail before manager Start()")
		Expect(strings.Contains(err.Error(), "cache")).To(BeTrue(),
			"expected cache error from cached client before Start(), got: %v", err)
	})
})
