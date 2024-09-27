package managedclusters

import (
	"context"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type ManagedClusterInfoCtrl struct {
	runtimeClient   client.Client
	inventoryClient transport.InventoryClient
	hubName         string
}

func (r *ManagedClusterInfoCtrl) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// log := r.Log.WithValues("mycustom", req.NamespacedName)

	// // Fetch the MyCustom resource
	// var myCustom mygroupv1.MyCustom
	// if err := r.Get(ctx, req.NamespacedName, &myCustom); err != nil {
	// 		if errors.IsNotFound(err) {
	// 				log.Info("MyCustom resource not found. Ignoring since it must have been deleted.")
	// 				return ctrl.Result{}, nil
	// 		}
	// 		log.Error(err, "Failed to get MyCustom")
	// 		return ctrl.Result{}, err
	// }

	// // Fetch a related Deployment (or any resource you're watching)
	// var deployment appsv1.Deployment
	// if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: "my-deployment"}, &deployment); err != nil {
	// 		if errors.IsNotFound(err) {
	// 				log.Info("Deployment resource not found.")
	// 				return ctrl.Result{}, nil
	// 		}
	// 		log.Error(err, "Failed to get Deployment")
	// 		return ctrl.Result{}, err
	// }

	// // Implement your reconciliation logic here

	return ctrl.Result{}, nil
}

func AddManagedClusterInfoCtrl(mgr ctrl.Manager, inventoryClient transport.InventoryClient) error {
	// Define a predicate to filter out events that don't need reconciliation
	clusterInfoPredicate := predicate.Funcs{
		// Only reconcile on Update events
		UpdateFunc: func(e event.UpdateEvent) bool {
			// oldObj := e.ObjectOld.(*clusterinfov1beta1.ManagedClusterInfo)
			// newObj := e.ObjectNew.(*clusterinfov1beta1.ManagedClusterInfo)
			// if oldObj.Generation == newObj.Generation {
			// 	return false
			// }
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterinfov1beta1.ManagedClusterInfo{}).
		WithEventFilter(clusterInfoPredicate).
		Complete(&ManagedClusterInfoCtrl{
			runtimeClient:   mgr.GetClient(),
			inventoryClient: inventoryClient,
			hubName:         statusconfig.GetLeafHubName(),
		})
}
