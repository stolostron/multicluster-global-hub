package transporter

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

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
		// this controller also will update the transport connection
		if config.GetKafkaResourceReady() && r.kafkaController == nil {
			r.kafkaController, err = protocol.StartKafkaController(ctx, r.Manager)
			if err != nil {
				return err
			}
			_, err := r.kafkaController.Reconcile(ctx, ctrl.Request{})
			if err != nil {
				return err
			}
		}
	case transport.SecretTransporter:
		trans = protocol.NewBYOTransporter(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      constants.GHTransportSecretName,
		}, r.GetClient())
		config.SetTransporter(trans)
		// all of hubs will get the same credential
		conn, err := trans.GetConnCredential("")
		if err != nil {
			return err
		}
		config.SetTransporterConn(conn)
	}
	return nil
}
