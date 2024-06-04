package manager

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transport/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	cfg           *rest.Config
	runtimeClient client.Client
	kubeClient    kubernetes.Interface
	runtimeMgr    ctrl.Manager
	namespace     = "default"
	ctx           context.Context
	cancel        context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	err := os.Setenv("POD_NAMESPACE", namespace)
	if err != nil {
		panic(err)
	}

	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{Scheme: config.GetRuntimeScheme()})
	if err != nil {
		panic(err)
	}
	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	runtimeMgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: config.GetRuntimeScheme(),
		LeaderElection:          true,
		LeaderElectionNamespace: namespace,
		LeaderElectionID:        "549a8911.open-cluster-management.io",
	})
	if err != nil {
		panic(err)
	}

	go func() {
		if err = runtimeMgr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	// run testings
	code := m.Run()

	cancel()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestManager(t *testing.T) {
	RegisterTestingT(t)

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				operatorconstants.AnnotationImageOverridesCM: "noexisting-cm",
			},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			EnableMetrics: true,
		},
	}
	Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())

	reconciler := NewManagerReconciler(runtimeMgr, kubeClient, &config.OperatorConfig{
		LogLevel:              "info",
		EnablePprof:           false,
		GlobalResourceEnabled: true,
	})

	// update the middleware configuration
	// storage
	_ = config.SetStorageConnection(&config.PostgresConnection{
		SuperuserDatabaseURI:    "test-url",
		ReadonlyUserDatabaseURI: "test-url",
		CACert:                  []byte("test-crt"),
	})
	config.SetDatabaseReady(true)

	// transport
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"bootstrap_server": []byte("localhost:test"),
			"ca.crt":           []byte("ca.crt"),
			"client.crt":       []byte("client.crt"),
			"client.key":       []byte("client.key"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	Expect(runtimeClient.Create(ctx, secret)).To(Succeed())
	trans := transporter.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, runtimeClient)
	config.SetTransporter(trans)

	err := reconciler.Reconcile(ctx, mgh)
	Expect(err).To(Succeed())

	// deployment
	deployment := &appsv1.Deployment{}
	err = runtimeClient.Get(ctx, types.NamespacedName{
		Name:      "multicluster-global-hub-manager",
		Namespace: mgh.Namespace,
	}, deployment)
	Expect(err).To(Succeed())

	// other objects
	transportTopic := trans.GenerateClusterTopic(transporter.GlobalHubClusterName)
	transportConn, err := trans.GetConnCredential(transporter.DefaultGlobalHubKafkaUser)
	Expect(err).To(Succeed())

	hohRenderer := renderer.NewHoHRenderer(fs)
	managerObjects, err := hohRenderer.Render("manifests", "", func(
		profile string,
	) (interface{}, error) {
		return ManagerVariables{
			Image:                  config.GetImage(config.GlobalHubManagerImageKey),
			Replicas:               2,
			ProxyImage:             config.GetImage(config.OauthProxyImageKey),
			ImagePullPolicy:        string(corev1.PullAlways),
			ImagePullSecret:        mgh.Spec.ImagePullSecret,
			ProxySessionSecret:     "testing",
			DatabaseURL:            base64.StdEncoding.EncodeToString([]byte("test-url")),
			PostgresCACert:         base64.StdEncoding.EncodeToString([]byte("test-crt")),
			KafkaClusterIdentity:   transportConn.Identity,
			KafkaCACert:            transportConn.CACert,
			KafkaClientCert:        transportConn.ClientCert,
			KafkaClientKey:         transportConn.ClientKey,
			KafkaBootstrapServer:   transportConn.BootstrapServer,
			KafkaConsumerTopic:     transportTopic.StatusTopic,
			KafkaProducerTopic:     transportTopic.SpecTopic,
			KafkaEventTopic:        transportTopic.EventTopic,
			MessageCompressionType: string(operatorconstants.GzipCompressType),
			TransportType:          string(transport.Kafka),
			Namespace:              commonutils.GetDefaultNamespace(),
			LeaseDuration:          "137",
			RenewDeadline:          "107",
			RetryPeriod:            "26",
			SkipAuth:               config.SkipAuth(mgh),
			SchedulerInterval:      config.GetSchedulerInterval(mgh),
			NodeSelector:           map[string]string{"foo": "bar"},
			Tolerations: []corev1.Toleration{
				{
					Key:      "dedicated",
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
					Value:    "infra",
				},
			},
			RetentionMonth:       18,
			StatisticLogInterval: config.GetStatisticLogInterval(),
			EnableGlobalResource: true,
			LaunchJobNames:       config.GetLaunchJobNames(mgh),
			EnablePprof:          false,
			LogLevel:             "info",
			Resources:            operatorutils.GetResources(operatorconstants.Manager, mgh.Spec.AdvancedConfig),
		}, nil
	})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		for _, desiredObj := range managerObjects {
			objLookupKey := types.NamespacedName{Name: desiredObj.GetName(), Namespace: desiredObj.GetNamespace()}
			foundObj := &unstructured.Unstructured{}
			foundObj.SetGroupVersionKind(desiredObj.GetObjectKind().GroupVersionKind())
			err := runtimeClient.Get(ctx, objLookupKey, foundObj)
			if err != nil {
				return err
			}
		}
		fmt.Printf("all manager resources(%d) are created as expected \n", len(managerObjects))
		return nil
	}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
}
