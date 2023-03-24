package hubofhubs

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func (r *MulticlusterGlobalHubReconciler) reconcileManager(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	log := ctrllog.FromContext(ctx)
	log.Info("reconciling manager")

	kafkaBootstrapServer, kafkaCACert, err := utils.GetKafkaConfig(ctx, r.KubeClient, mgh)
	if err != nil {
		return err
	}
	log.Info("retrieved kafka bootstrap server and CA cert from secret")
	if e := condition.SetConditionTransportInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE); e != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, e)
	}

	// generate random session secret for oauth-proxy
	proxySessionSecret, err := utils.GeneratePassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate random session secret for oauth-proxy: %v", err)
	}

	messageCompressionType := string(mgh.Spec.MessageCompressionType)
	if messageCompressionType == "" {
		messageCompressionType = string(operatorv1alpha2.GzipCompressType)
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
			KafkaBootstrapServer   string
			MessageCompressionType string
			TransportType          string
			TransportFormat        string
			Namespace              string
			LeaseDuration          string
			RenewDeadline          string
			RetryPeriod            string
		}{
			Image:                  config.GetImage(config.GlobalHubManagerImageKey),
			ProxyImage:             config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:        mgh.Spec.ImagePullSecret,
			ImagePullPolicy:        string(imagePullPolicy),
			ProxySessionSecret:     proxySessionSecret,
			DBSecret:               mgh.Spec.DataLayer.LargeScale.Postgres.Name,
			KafkaCACert:            kafkaCACert,
			KafkaBootstrapServer:   kafkaBootstrapServer,
			MessageCompressionType: messageCompressionType,
			TransportType:          string(transport.Kafka),
			TransportFormat:        string(mgh.Spec.DataLayer.LargeScale.Kafka.TransportFormat),
			Namespace:              config.GetDefaultNamespace(),
			LeaseDuration:          strconv.Itoa(r.LeaderElection.LeaseDuration),
			RenewDeadline:          strconv.Itoa(r.LeaderElection.RenewDeadline),
			RetryPeriod:            strconv.Itoa(r.LeaderElection.RetryPeriod),
		}, nil
	})
	if err != nil {
		return err
	}
	if err = r.manipulateObj(ctx, hohDeployer, mapper, managerObjects, mgh,
		condition.SetConditionManagerDeployed, log); err != nil {
		return err
	}

	return nil
}

func (r *MulticlusterGlobalHubReconciler) manipulateObj(ctx context.Context, hohDeployer deployer.Deployer,
	mapper *restmapper.DeferredDiscoveryRESTMapper, objs []*unstructured.Unstructured,
	mgh *operatorv1alpha2.MulticlusterGlobalHub, setConditionFunc condition.SetConditionFunc,
	log logr.Logger,
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

		log.Info("Creating or updating object", "object", obj)
		if err := hohDeployer.Deploy(obj); err != nil {
			if setConditionFunc != nil {
				conditionError := setConditionFunc(ctx, r.Client, mgh, condition.CONDITION_STATUS_FALSE)
				if conditionError != nil {
					return condition.FailToSetConditionError(
						condition.CONDITION_STATUS_FALSE, conditionError)
				}
			}
			return err
		}
	}

	if setConditionFunc != nil {
		if conditionError := setConditionFunc(ctx, r.Client, mgh,
			condition.CONDITION_STATUS_TRUE); conditionError != nil {
			return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, conditionError)
		}
	}
	return nil
}
