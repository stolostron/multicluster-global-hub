package hubofhubs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
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
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	log := ctrllog.FromContext(ctx)
	log.Info("reconciling manager")

	kafkaBootstrapServer, kafkaCACert, err := utils.GetKafkaConfig(ctx, r.KubeClient, mgh)
	if err != nil {
		return err
	}
	log.Info("retrieved kafka bootstrap server and CA cert from secret")
	if e := condition.SetConditionTransportInit(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); e != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, e)
	}

	// generate random session secret for oauth-proxy
	proxySessionSecret, err := config.GetOauthSessionSecret()
	if err != nil {
		return fmt.Errorf("failed to get random session secret for oauth-proxy: %v", err)
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
	imagePullSecret := operatorconstants.DefaultImagePullSecretName
	if mgh.Spec.ImagePullSecret != "" {
		imagePullSecret = mgh.Spec.ImagePullSecret
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
			NodeSelector           map[string]string
			Tolerations            []corev1.Toleration
		}{
			Image:                  config.GetImage(config.GlobalHubManagerImageKey),
			ProxyImage:             config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:        imagePullSecret,
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

	if err := r.updateDeploymentStatus(ctx, operatorconstants.GHManagerDeploymentName, mgh,
		condition.CONDITION_TYPE_MANAGER_AVAILABLE, log); err != nil {
		return fmt.Errorf("failed to update manager deployment status: %v", err)
	}
	return nil
}

func (r *MulticlusterGlobalHubReconciler) updateDeploymentStatus(ctx context.Context, deployName string,
	mgh *operatorv1alpha2.MulticlusterGlobalHub, conditionType string, log logr.Logger,
) error {
	targetCondition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionFalse,
		Reason:             "DeployedButNotReady",
		Message:            "Deployment is created but not ready yet",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	deployment := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      deployName,
		Namespace: config.GetDefaultNamespace(),
	}, deployment); err != nil {
		log.Error(err, "failed to get deployment")
	}
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			targetCondition.Status = metav1.ConditionStatus(cond.Status)
			targetCondition.Reason = cond.Reason
			targetCondition.Message = cond.Message
			targetCondition.LastTransitionTime = cond.LastTransitionTime
		}
	}
	if condition.ContainConditionStatusReason(mgh, conditionType, targetCondition.Reason, targetCondition.Status) {
		return nil
	}
	if err := condition.UpdateCondition(ctx, r.Client, mgh, targetCondition); err != nil {
		return err
	}
	return nil
}

func (r *MulticlusterGlobalHubReconciler) manipulateObj(ctx context.Context, hohDeployer deployer.Deployer,
	mapper *restmapper.DeferredDiscoveryRESTMapper, objs []*unstructured.Unstructured,
	mgh *operatorv1alpha2.MulticlusterGlobalHub, log logr.Logger,
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
