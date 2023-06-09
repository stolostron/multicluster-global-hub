package hubofhubs

import (
	"context"
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

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func (r *MulticlusterGlobalHubReconciler) reconcileManager(ctx context.Context,
	mgh *operatorv1alpha3.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("manager")

	log.Info("retrieving transport secret for the manager", "name",
		operatorconstants.GHTransportSecretName)
	kafkaBootstrapServer, kafkaCACert, kafkaClientCert, kafkaClientKey, err := utils.GetKafkaConfig(ctx,
		r.KubeClient, mgh.Namespace, operatorconstants.GHTransportSecretName)
	if err != nil {
		return err
	}
	if e := condition.SetConditionTransportInit(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); e != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, e)
	}

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

	managerObjects, err := hohRenderer.Render("manifests/manager", "", func(profile string) (interface{}, error) {
		return struct {
			Image                  string
			ProxyImage             string
			ImagePullSecret        string
			ImagePullPolicy        string
			ProxySessionSecret     string
			DBSecret               string
			KafkaCACert            string
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
			NodeSelector           map[string]string
			Tolerations            []corev1.Toleration
		}{
			Image:                  config.GetImage(config.GlobalHubManagerImageKey),
			ProxyImage:             config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:        mgh.Spec.ImagePullSecret,
			ImagePullPolicy:        string(imagePullPolicy),
			ProxySessionSecret:     proxySessionSecret,
			DBSecret:               operatorconstants.GHStorageSecretName,
			KafkaCACert:            kafkaCACert,
			KafkaClientCert:        kafkaClientCert,
			KafkaClientKey:         kafkaClientKey,
			KafkaBootstrapServer:   kafkaBootstrapServer,
			MessageCompressionType: string(operatorconstants.GzipCompressType),
			TransportType:          string(transport.Kafka),
			TransportFormat:        string(mgh.Spec.DataLayer.LargeScale.Kafka.TransportFormat),
			Namespace:              config.GetDefaultNamespace(),
			LeaseDuration:          strconv.Itoa(r.LeaderElection.LeaseDuration),
			RenewDeadline:          strconv.Itoa(r.LeaderElection.RenewDeadline),
			RetryPeriod:            strconv.Itoa(r.LeaderElection.RetryPeriod),
			SchedulerInterval:      config.GetSchedulerInterval(mgh),
			NodeSelector:           mgh.Spec.NodeSelector,
			Tolerations:            mgh.Spec.Tolerations,
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
	mgh *operatorv1alpha3.MulticlusterGlobalHub, log logr.Logger,
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
