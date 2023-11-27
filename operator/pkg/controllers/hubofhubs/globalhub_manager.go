package hubofhubs

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func (r *MulticlusterGlobalHubReconciler) reconcileManager(ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("manager")

	// generate random session secret for oauth-proxy
	proxySessionSecret, err := config.GetOauthSessionSecret()
	if err != nil {
		return fmt.Errorf("failed to get random session secret for oauth-proxy: %v", err)
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.Client)

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
	// if parsing fails, then set the error message to the condition
	if err != nil {
		e := condition.SetConditionDataRetention(ctx, r.Client, mgh, condition.CONDITION_STATUS_FALSE, err.Error())
		if e != nil {
			return condition.FailToSetConditionError(condition.CONDITION_TYPE_RETENTION_PARSED, e)
		}
		return fmt.Errorf("failed to parse month retention: %v", err)
	}
	if months < 1 {
		months = 1
	}
	// If parsing succeeds, update the MGH status and message of the condition if they are not set or changed
	msg := fmt.Sprintf("The data will be kept in the database for %d months.", months)
	if e := condition.SetConditionDataRetention(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE, msg); e != nil {
		return condition.FailToSetConditionError(condition.CONDITION_TYPE_RETENTION_PARSED, err)
	}

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}

	globalTopic := transport.GetTopics(transport.GlobalHubTopicIdentity)

	managerObjects, err := hohRenderer.Render("manifests/manager", "", func(profile string) (interface{}, error) {
		return ManagerVariables{
			Image:              config.GetImage(config.GlobalHubManagerImageKey),
			Replicas:           replicas,
			ProxyImage:         config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:    mgh.Spec.ImagePullSecret,
			ImagePullPolicy:    string(imagePullPolicy),
			ProxySessionSecret: proxySessionSecret,
			DatabaseURL: base64.StdEncoding.EncodeToString(
				[]byte(r.MiddlewareConfig.StorageConn.SuperuserDatabaseURI)),
			PostgresCACert:         base64.StdEncoding.EncodeToString(r.MiddlewareConfig.StorageConn.CACert),
			KafkaCACert:            r.MiddlewareConfig.TransportConn.CACert,
			KafkaClientCert:        r.MiddlewareConfig.TransportConn.ClientCert,
			KafkaClientKey:         r.MiddlewareConfig.TransportConn.ClientKey,
			KafkaBootstrapServer:   r.MiddlewareConfig.TransportConn.BootstrapServer,
			KafkaConsumerTopic:     globalTopic.StatusTopic,
			KafkaProducerTopic:     globalTopic.SpecTopic,
			KafkaEventTopic:        globalTopic.EventTopic,
			MessageCompressionType: string(operatorconstants.GzipCompressType),
			TransportType:          string(transport.Kafka),
			TransportFormat:        string(v1alpha4.CloudEvents),
			Namespace:              config.GetDefaultNamespace(),
			LeaseDuration:          strconv.Itoa(r.LeaderElection.LeaseDuration),
			RenewDeadline:          strconv.Itoa(r.LeaderElection.RenewDeadline),
			RetryPeriod:            strconv.Itoa(r.LeaderElection.RetryPeriod),
			SchedulerInterval:      config.GetSchedulerInterval(mgh),
			SkipAuth:               config.SkipAuth(mgh),
			LaunchJobNames:         config.GetLaunchJobNames(mgh),
			NodeSelector:           mgh.Spec.NodeSelector,
			Tolerations:            mgh.Spec.Tolerations,
			RetentionMonth:         months,
			StatisticLogInterval:   config.GetStatisticLogInterval(),
			EnableGlobalResource:   r.EnableGlobalResource,
			LogLevel:               r.LogLevel,
		}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to render manager objects: %v", err)
	}
	if err = r.manipulateObj(ctx, hohDeployer, mapper, managerObjects, mgh, log); err != nil {
		return fmt.Errorf("failed to create/update manager objects: %v", err)
	}

	log.Info("manager objects are created/updated successfully")
	return nil
}

func (r *MulticlusterGlobalHubReconciler) manipulateObj(ctx context.Context, hohDeployer deployer.Deployer,
	mapper *restmapper.DeferredDiscoveryRESTMapper, objs []*unstructured.Unstructured,
	mgh *v1alpha4.MulticlusterGlobalHub, log logr.Logger,
) error {
	// manipulate the object
	for _, obj := range objs {
		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			log.Error(err, "failed to find mapping for resource", "kind", obj.GetKind(),
				"namespace", obj.GetNamespace(), "name", obj.GetName())
			return err
		}

		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// for namespaced resource, set ownerreference of controller
			if err := controllerutil.SetControllerReference(mgh, obj, r.Scheme); err != nil {
				log.Error(err, "failed to set controller reference", "kind", obj.GetKind(),
					"namespace", obj.GetNamespace(), "name", obj.GetName())
				return err
			}
		}

		// set owner labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.GlobalHubOwnerLabelKey] = constants.GHOperatorOwnerLabelVal
		obj.SetLabels(labels)

		if err := hohDeployer.Deploy(obj); err != nil {
			return err
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
	KafkaCACert            string
	KafkaConsumerTopic     string
	KafkaProducerTopic     string
	KafkaEventTopic        string
	KafkaClientCert        string
	KafkaClientKey         string
	KafkaBootstrapServer   string
	MessageCompressionType string
	TransportType          string
	TransportFormat        string
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
	LogLevel               string
}
