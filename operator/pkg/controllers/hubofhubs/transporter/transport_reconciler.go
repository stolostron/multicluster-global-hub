package transporter

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type TransportReconciler struct {
	ctrl.Manager
	kafkaController *protocol.KafkaController
	transporter     transport.Transporter
}

func NewTransportReconciler(mgr ctrl.Manager) *TransportReconciler {
	return &TransportReconciler{Manager: mgr}
}

// Resources reconcile the transport resources and also update transporter on the configuration
func (r *TransportReconciler) Reconcile(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (err error) {
	defer func() {
		config.UpdateTransportCondition(config.CONDITION_TRANSPORT_REASON_PROTOCOL, err)
	}()
	// set the transporter
	switch config.TransporterProtocol() {
	case transport.StrimziTransporter:
		// initialize strimzi
		// kafkaCluster, it will be blocking until the status is ready
		if r.transporter == nil {
			r.transporter = protocol.NewStrimziTransporter(
				r.Manager,
				mgh,
				protocol.WithContext(ctx),
				protocol.WithCommunity(operatorutils.IsCommunityMode()),
			)
			if err = r.transporter.EnsureKafka(); err != nil {
				return err
			}
		} else {
			strimziTran, ok := (r.transporter).(*protocol.StrimziTransporter)
			if !ok {
				return fmt.Errorf("the transport type should be Strimzi")
			}
			strimziTran.Update(mgh)
		}
		// update the transporter
		config.SetTransporter(r.transporter)

		// this controller also will update the transport connection
		if config.GetKafkaResourceReady() && r.kafkaController == nil {
			r.kafkaController, err = protocol.StartKafkaController(ctx, r.Manager, r.transporter)
			if err != nil {
				return err
			}
		}
	case transport.SecretTransporter:
		if r.transporter == nil {
			r.transporter = protocol.NewBYOTransporter(ctx, types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      constants.GHTransportSecretName,
			}, r.GetClient())
			config.SetTransporter(r.transporter)
			// all of hubs will get the same credential
			var conn *transport.KafkaConnCredential
			conn, err = r.transporter.GetConnCredential("")
			if err != nil {
				return err
			}
			config.SetTransporterConn(conn)
		}
	}
	return nil
}
