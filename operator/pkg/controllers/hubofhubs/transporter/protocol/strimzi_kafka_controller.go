// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var manifests embed.FS

// KafkaController reconciles the kafka crd
type KafkaController struct {
	ctrl.Manager
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	mgh := &v1alpha4.MulticlusterGlobalHub{}
	if err := r.GetClient().Get(ctx, config.GetMGHNamespacedName(), mgh); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// kafkaCluster
	trans, err := NewStrimziTransporter(
		r.GetClient(),
		mgh,
		WithContext(ctx),
		WithCommunity(operatorutils.IsCommunityMode()),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	// kafka metrics, monitor, global hub kafkaTopic and kafkaUser
	if err := r.renderKafkaResources(mgh); err != nil {
		return ctrl.Result{}, err
	}

	// update the transporter
	config.SetTransporter(trans)

	// update the transport connection
	conn, err := waitTransportConn(ctx, trans, DefaultGlobalHubKafkaUserName)
	if err != nil {
		return ctrl.Result{}, err
	}
	config.SetTransporterConn(conn)

	return ctrl.Result{}, nil
}

var kafkaPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == utils.GetDefaultNamespace()
	},
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func StartKafkaController(ctx context.Context, mgr ctrl.Manager) (*KafkaController, error) {
	r := &KafkaController{Manager: mgr}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("kafka_controller").
		Watches(&v1alpha4.MulticlusterGlobalHub{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(mghPred)).
		Watches(&kafkav1beta2.Kafka{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaUser{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaTopic{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Complete(r)
	if err != nil {
		return nil, err
	}
	klog.Info("kafka controller is started")
	return r, nil
}

func waitTransportConn(ctx context.Context, trans *strimziTransporter, kafkaUserSecret string) (
	*transport.ConnCredential, error,
) {
	// set transporter connection
	var conn *transport.ConnCredential
	var err error
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			conn, err = trans.getConnCredentailByCluster()
			if err != nil {
				klog.Info("waiting the kafka cluster credential to be ready...", "message", err.Error())
				return false, err
			}

			if err := trans.loadUserCredentail(kafkaUserSecret, conn); err != nil {
				klog.Info("waiting the kafka user credential to be ready...", "message", err.Error())
				return false, err
			}
			return true, nil
		})
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// renderKafkaMetricsResources renders the kafka podmonitor and metrics, and kafkaUser and kafkaTopic for global hub
func (r *KafkaController) renderKafkaResources(mgh *v1alpha4.MulticlusterGlobalHub) error {
	statusTopic := config.GetRawStatusTopic()
	statusPlaceholderTopic := config.GetRawStatusTopic()
	topicParttern := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	if strings.Contains(config.GetRawStatusTopic(), "*") {
		statusTopic = strings.Replace(config.GetRawStatusTopic(), "*", "", -1)
		statusPlaceholderTopic = strings.Replace(config.GetRawStatusTopic(), "*", "global-hub", -1)
		topicParttern = kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypePrefix
	}
	// render the kafka objects
	kafkaRenderer, kafkaDeployer := renderer.NewHoHRenderer(manifests), deployer.NewHoHDeployer(r.GetClient())
	kafkaObjects, err := kafkaRenderer.Render("manifests", "",
		func(profile string) (interface{}, error) {
			return struct {
				EnableMetrics          bool
				Namespace              string
				KafkaCluster           string
				GlobalHubKafkaUser     string
				SpecTopic              string
				StatusTopic            string
				StatusTopicParttern    string
				StatusPlaceholderTopic string
				TopicPartition         int32
				TopicReplicas          int32
			}{
				EnableMetrics:          mgh.Spec.EnableMetrics,
				Namespace:              mgh.GetNamespace(),
				KafkaCluster:           KafkaClusterName,
				GlobalHubKafkaUser:     DefaultGlobalHubKafkaUserName,
				SpecTopic:              config.GetSpecTopic(),
				StatusTopic:            statusTopic,
				StatusTopicParttern:    string(topicParttern),
				StatusPlaceholderTopic: statusPlaceholderTopic,
				TopicPartition:         DefaultPartition,
				TopicReplicas:          DefaultPartitionReplicas,
			}, nil
		})
	if err != nil {
		return fmt.Errorf("failed to render kafka manifests: %w", err)
	}
	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	if err = operatorutils.ManipulateGlobalHubObjects(kafkaObjects, mgh, kafkaDeployer, mapper,
		r.Manager.GetScheme()); err != nil {
		return fmt.Errorf("failed to create/update kafka objects: %w", err)
	}
	return nil
}
