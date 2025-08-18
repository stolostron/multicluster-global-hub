package jobs_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"open-cluster-management.io/api/client/cluster/clientset/versioned/scheme"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsubv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	runtimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

func TestJobs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Jobs Suite")
}

var (
	testenv                    *envtest.Environment
	cfg                        *rest.Config
	runtimeClient              client.Client
	runtimeClientWithoutScheme client.Client
	ctx                        context.Context
	cancel                     context.CancelFunc
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "test", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
	}

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel = context.WithCancel(context.Background())

	runtimeClientWithoutScheme, err = client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	Expect(addToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	runtimeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Expect(testenv.Stop()).To(Succeed())
})

func addToScheme(runtimeScheme *runtime.Scheme) error {
	schemeBuilders := []*runtimescheme.Builder{
		policyv1.SchemeBuilder,
		placementrulev1.SchemeBuilder,
		appsubv1alpha1.SchemeBuilder,
		mchv1.SchemeBuilder,
		appv1beta1.SchemeBuilder,
		appsubv1.SchemeBuilder,
		chnv1.SchemeBuilder,
	} // add schemes

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(runtimeScheme); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}
	if err := apiregistrationv1.AddToScheme(runtimeScheme); err != nil {
		return err
	}

	return nil
}
