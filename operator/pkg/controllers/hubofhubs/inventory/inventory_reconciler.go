package inventory

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
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
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
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
