package transporter

import (
	"context"
	"embed"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

//go:embed manifests
var manifests embed.FS

type TransportReconciler struct {
	ctrl.Manager
	kafkaController *protocol.KafkaController
}

func NewTransportReconciler(mgr ctrl.Manager) *TransportReconciler {
	return &TransportReconciler{Manager: mgr}
}

// Resources reconcile the transport resources and also update transporter on the configuration
func (r *TransportReconciler) Reconcile(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (err error) {
	// set the transporter
	var trans transport.Transporter
	switch config.TransporterProtocol() {
	case transport.StrimziTransporter:
		err = r.reconcileKafkaResources(ctx, mgh)
	case transport.SecretTransporter:
		trans = protocol.NewBYOTransporter(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      constants.GHTransportSecretName,
		}, r.GetClient())
		config.SetTransporter(trans)
		err = loadTransportConn(ctx, trans)
	}
	if err != nil {
		return err
	}

	// set kafka controller
	if config.GetKafkaResourceReady() && r.kafkaController == nil {
		r.kafkaController, err = protocol.StartKafkaController(r.Manager, r.reconcileKafkaResources)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *TransportReconciler) reconcileKafkaResources(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error {
	// kafka metrics
	err := r.renderKafkaMetrics(mgh)
	if err != nil {
		return err
	}

	// kafkaCluster, kafkaUser, kafkaTopics
	trans, err := protocol.NewStrimziTransporter(
		r.GetClient(),
		mgh,
		protocol.WithContext(ctx),
		protocol.WithCommunity(operatorutils.IsCommunityMode()),
	)
	if err != nil {
		return err
	}

	// create the user to connect the transport instance
	err = trans.CreateAndUpdateUser(protocol.DefaultGlobalHubKafkaUser)
	if err != nil {
		return err
	}
	// create global hub topics, create the status.global, spec and event topics
	// it's a placeholder for the manager to subscribe the `^status.*`
	topics := trans.GenerateClusterTopic(protocol.GlobalHubClusterName)
	err = trans.CreateAndUpdateTopic(topics)
	if err != nil {
		return err
	}
	// grant permission: read - status,event, write - spec;
	err = trans.GrantRead(protocol.DefaultGlobalHubKafkaUser, topics.EventTopic)
	if err != nil {
		return err
	}
	err = trans.GrantRead(protocol.DefaultGlobalHubKafkaUser, topics.StatusTopic)
	if err != nil {
		return err
	}
	err = trans.GrantWrite(protocol.DefaultGlobalHubKafkaUser, topics.SpecTopic)
	if err != nil {
		return err
	}

	config.SetTransporter(trans)
	return loadTransportConn(ctx, trans)
}

func loadTransportConn(ctx context.Context, trans transport.Transporter) error {
	// set transporter connection
	var conn *transport.ConnCredential
	var err error
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			conn, err = trans.GetConnCredential(protocol.DefaultGlobalHubKafkaUser)
			if err != nil {
				klog.Info("waiting the kafka connection credential to be ready...", "message", err.Error())
				return false, err
			}
			return true, nil
		})
	if err != nil {
		return err
	}
	config.SetTransporterConn(conn)
	return nil
}

// renderKafkaMetricsResources renders the kafka podmonitor and metrics
func (r *TransportReconciler) renderKafkaMetrics(mgh *v1alpha4.MulticlusterGlobalHub) error {
	if (!config.IsBYOKafka()) && mgh.Spec.EnableMetrics {
		// render the kafka objects
		kafkaRenderer, kafkaDeployer := renderer.NewHoHRenderer(manifests), deployer.NewHoHDeployer(r.GetClient())
		kafkaObjects, err := kafkaRenderer.Render("manifests", "",
			func(profile string) (interface{}, error) {
				return struct {
					Namespace string
				}{
					Namespace: mgh.GetNamespace(),
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
	}
	return nil
}
