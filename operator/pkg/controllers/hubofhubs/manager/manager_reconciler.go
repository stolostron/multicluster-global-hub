package manager

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var fs embed.FS

var (
	storageConnectionCache   *config.PostgresConnection
	transportConnectionCache *transport.ConnCredential
)

type ManagerReconciler struct {
	ctrl.Manager
	runtimeClient  client.Client
	kubeClient     kubernetes.Interface
	operatorConfig *config.OperatorConfig
}

func NewManagerReconciler(mgr ctrl.Manager, kubeClient kubernetes.Interface, conf *config.OperatorConfig,
) *ManagerReconciler {
	return &ManagerReconciler{
		Manager:        mgr,
		runtimeClient:  mgr.GetClient(),
		kubeClient:     kubeClient,
		operatorConfig: conf,
	}
}

func (r *ManagerReconciler) Reconcile(ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	// generate random session secret for oauth-proxy
	proxySessionSecret, err := config.GetOauthSessionSecret()
	if err != nil {
		return fmt.Errorf("failed to get random session secret for oauth-proxy: %v", err)
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	// dataRetention should at least be 1 month, otherwise it will deleted the current month partitions and records
	months, err := commonutils.ParseRetentionMonth(mgh.Spec.DataLayer.Postgres.Retention)
	if err != nil {
		return fmt.Errorf("failed to parse month retention: %v", err)
	}
	if months < 1 {
		months = 1
	}

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}

	transportConn := config.GetTransporterConn()
	if transportConn == nil {
		return fmt.Errorf("the transport connection(%s) must not be empty", transportConn)
	}

	if updateTransportConn(transportConn) {
		err = r.ensureTransportSecret(ctx, mgh.Namespace, transportConn)
		if err != nil {
			return fmt.Errorf("failed to create/update manager secret: %w", err)
		}
	}

	storageConn := config.GetStorageConnection()
	if storageConn == nil || !config.GetDatabaseReady() {
		return fmt.Errorf("the storage connection or database isn't ready")
	}

	if updateStorageConn(storageConn) {
		err = commonutils.RestartPod(ctx, r.kubeClient, mgh.Namespace, constants.ManagerDeploymentName)
		if err != nil {
			return fmt.Errorf("failed to restart manager pod: %w", err)
		}
	}

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		return fmt.Errorf("failed to get the electionConfig %w", err)
	}

	managerObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return ManagerVariables{
			Image:              config.GetImage(config.GlobalHubManagerImageKey),
			Replicas:           replicas,
			ProxyImage:         config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:    mgh.Spec.ImagePullSecret,
			ImagePullPolicy:    string(imagePullPolicy),
			ProxySessionSecret: proxySessionSecret,
			DatabaseURL: base64.StdEncoding.EncodeToString(
				[]byte(storageConn.SuperuserDatabaseURI)),
			PostgresCACert:         base64.StdEncoding.EncodeToString(storageConn.CACert),
			Namespace:              mgh.Namespace,
			MessageCompressionType: string(operatorconstants.GzipCompressType),
			LeaseDuration:          strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:          strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:            strconv.Itoa(electionConfig.RetryPeriod),
			SchedulerInterval:      config.GetSchedulerInterval(mgh),
			SkipAuth:               config.SkipAuth(mgh),
			LaunchJobNames:         config.GetLaunchJobNames(mgh),
			NodeSelector:           mgh.Spec.NodeSelector,
			Tolerations:            mgh.Spec.Tolerations,
			RetentionMonth:         months,
			StatisticLogInterval:   config.GetStatisticLogInterval(),
			EnableGlobalResource:   r.operatorConfig.GlobalResourceEnabled,
			EnablePprof:            r.operatorConfig.EnablePprof,
			LogLevel:               r.operatorConfig.LogLevel,
			Resources:              utils.GetResources(operatorconstants.Manager, mgh.Spec.AdvancedConfig),
			WithACM:                config.IsACMResourceReady(),
		}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to render manager objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(managerObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		return fmt.Errorf("failed to create/update manager objects: %v", err)
	}
	return nil
}

func updateTransportConn(conn *transport.ConnCredential) bool {
	if conn == nil {
		return false
	}
	if transportConnectionCache == nil || !reflect.DeepEqual(conn, transportConnectionCache) {
		transportConnectionCache = conn
		return true
	}
	return false
}

func updateStorageConn(conn *config.PostgresConnection) bool {
	if conn == nil {
		return false
	}
	if storageConnectionCache == nil || !reflect.DeepEqual(conn, storageConnectionCache) {
		storageConnectionCache = conn
		return true
	}
	return false
}

func (r *ManagerReconciler) ensureTransportSecret(ctx context.Context, namespace string,
	conn *transport.ConnCredential,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      constants.GHManagerTransportSecret,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	controller.LoadDataToSecret(secret, conn, &transport.ClusterTopic{
		StatusTopic: config.FuzzyStatusTopic(),
		SpecTopic:   config.GetSpecTopic(),
	}, "")

	// Try to get the existing secret
	existingSecret := &corev1.Secret{}
	err := r.runtimeClient.Get(ctx, client.ObjectKeyFromObject(secret), existingSecret)
	if err != nil && errors.IsNotFound(err) {
		klog.Infof("create the manager secret: %s", secret.Name)
		if err := r.runtimeClient.Create(ctx, secret); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if !reflect.DeepEqual(existingSecret.Data, secret.Data) {
			existingSecret.Data = secret.Data
			klog.Infof("update the manager secret: %s", secret.Name)
			if err := r.runtimeClient.Update(ctx, existingSecret); err != nil {
				return err
			}
		}
	}
	return nil
}

type ManagerVariables struct {
	Image                  string
	Replicas               int32
	ProxyImage             string
	ImagePullSecret        string
	ImagePullPolicy        string
	ProxySessionSecret     string
	DatabaseURL            string
	PostgresCACert         string
	MessageCompressionType string
	Namespace              string
	LeaseDuration          string
	RenewDeadline          string
	RetryPeriod            string
	SchedulerInterval      string
	SkipAuth               bool
	LaunchJobNames         string
	NodeSelector           map[string]string
	Tolerations            []corev1.Toleration
	RetentionMonth         int
	StatisticLogInterval   string
	EnableGlobalResource   bool
	EnablePprof            bool
	LogLevel               string
	Resources              *corev1.ResourceRequirements
	WithACM                bool
}
