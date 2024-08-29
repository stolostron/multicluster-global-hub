package inventory

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"net/url"
	"reflect"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	certctrl "github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

//go:embed manifests
var fs embed.FS

type InventoryReconciler struct {
	kubeClient kubernetes.Interface
	ctrl.Manager
	log logr.Logger
}

func NewInventoryReconciler(mgr ctrl.Manager, kubeClient kubernetes.Interface) *InventoryReconciler {
	return &InventoryReconciler{
		log:        ctrl.Log.WithName("global-hub-inventory"),
		Manager:    mgr,
		kubeClient: kubeClient,
	}
}

func (r *InventoryReconciler) Reconcile(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	// start certificate controller
	certctrl.Start(ctx, r.GetClient())

	// Need to create route so that the cert can use it
	if err := createUpdateInventoryRoute(ctx, r.GetClient(), mgh); err != nil {
		return err
	}

	// create inventory certs
	if err := certctrl.CreateInventoryCerts(ctx, r.GetClient(), r.GetScheme(), mgh); err != nil {
		return err
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

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == globalhubv1alpha4.HAHigh {
		replicas = 2
	}

	storageConn := config.GetStorageConnection()
	if storageConn == nil || !config.GetDatabaseReady() {
		return fmt.Errorf("the database isn't ready")
	}

	postgresURI, err := url.Parse(string(storageConn.SuperuserDatabaseURI))
	if err != nil {
		return err
	}
	postgresPassword, ok := postgresURI.User.Password()
	if !ok {
		return fmt.Errorf("failed to get password from database_uri: %s", postgresURI)
	}

	inventoryObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return struct {
			Image            string
			Replicas         int32
			ImagePullSecret  string
			ImagePullPolicy  string
			PostgresHost     string
			PostgresPort     string
			PostgresUser     string
			PostgresPassword string
			PostgresCACert   string
			Namespace        string
			NodeSelector     map[string]string
			Tolerations      []corev1.Toleration
		}{
			Image:            config.GetImage(config.InventoryImageKey),
			Replicas:         replicas,
			ImagePullSecret:  mgh.Spec.ImagePullSecret,
			ImagePullPolicy:  string(imagePullPolicy),
			PostgresHost:     postgresURI.Hostname(),
			PostgresPort:     postgresURI.Port(),
			PostgresUser:     postgresURI.User.Username(),
			PostgresPassword: postgresPassword,
			PostgresCACert:   base64.StdEncoding.EncodeToString(storageConn.CACert),
			Namespace:        mgh.Namespace,
			NodeSelector:     mgh.Spec.NodeSelector,
			Tolerations:      mgh.Spec.Tolerations,
		}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to render inventory objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(inventoryObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		return fmt.Errorf("failed to create/update inventory objects: %v", err)
	}
	return nil
}

func newInventoryRoute(mgh *globalhubv1alpha4.MulticlusterGlobalHub) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InventoryRouteName,
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				"name": constants.InventoryRouteName,
			},
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("http-server"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: constants.InventoryRouteName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}
}

func createUpdateInventoryRoute(ctx context.Context, c client.Client,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	existingRoute := &routev1.Route{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      constants.InventoryRouteName,
		Namespace: mgh.Namespace,
	}, existingRoute)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, newInventoryRoute(mgh))
		}
		return err
	}

	desiredRoute := newInventoryRoute(mgh)

	updatedRoute := &routev1.Route{}
	err = operatorutils.MergeObjects(existingRoute, desiredRoute, updatedRoute)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(updatedRoute.Spec, existingRoute.Spec) {
		return c.Update(ctx, updatedRoute)
	}

	return nil
}
