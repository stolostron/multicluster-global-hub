package hubofhubs

import (
	"context"
	"fmt"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

func (r *MulticlusterGlobalHubReconciler) reconcileGrafana(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	log := ctrllog.FromContext(ctx)
	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_GRAFANA_INIT, condition.CONDITION_STATUS_TRUE) {
		log.Info("Grafana has initialized")
		return nil
	}

	log.Info("Grafana initializing")
	// generate random session secret for oauth-proxy
	proxySessionSecret, err := utils.GeneratePassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate random session secret for grafana oauth-proxy: %v", err)
	}

	// get the grafana data source
	postgresSecret, err := r.KubeClient.CoreV1().Secrets(config.GetDefaultNamespace()).Get(ctx,
		mgh.Spec.DataLayer.LargeScale.Postgres.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	postgresURI := string(postgresSecret.Data["database_uri"])
	objURI, err := url.Parse(postgresURI)
	if err != nil {
		return err
	}
	password, ok := objURI.User.Password()
	if !ok {
		return fmt.Errorf("failed to get password from database_uri: %s", postgresURI)
	}

	caCert := string(postgresSecret.Data["ca.crt"])
	if caCert == "" {
		return fmt.Errorf("failed to get ca.crt from secret: %s", mgh.Spec.DataLayer.LargeScale.Postgres.Name)
	}
	clientCert := string(postgresSecret.Data["tls.crt"])
	if clientCert == "" {
		return fmt.Errorf("failed to get tls.crt from secret: %s", mgh.Spec.DataLayer.LargeScale.Postgres.Name)
	}
	clientKey := string(postgresSecret.Data["tls.key"])
	if clientKey == "" {
		return fmt.Errorf("failed to get tls.key from secret: %s", mgh.Spec.DataLayer.LargeScale.Postgres.Name)
	}

	// get the grafana objects
	grafanaRenderer, grafanaDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.Client)
	grafanaObjects, err := grafanaRenderer.Render("manifests/grafana", "", func(profile string) (interface{}, error) {
		return struct {
			Namespace            string
			SessionSecret        string
			ProxyImage           string
			POSTGRES_HOST        string
			POSTGRES_USER        string
			POSTGRES_PASSWORD    string
			POSTGRES_CA_CERT     string
			POSTGRES_CLIENT_CERT string
			POSTGRES_CLIENT_KEY  string
		}{
			Namespace:            config.GetDefaultNamespace(),
			SessionSecret:        proxySessionSecret,
			ProxyImage:           config.GetImage("oauth_proxy"),
			POSTGRES_HOST:        objURI.Host,
			POSTGRES_USER:        objURI.User.Username(),
			POSTGRES_PASSWORD:    password,
			POSTGRES_CA_CERT:     caCert,
			POSTGRES_CLIENT_CERT: clientCert,
			POSTGRES_CLIENT_KEY:  clientKey,
		}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to render grafana manifests: %w", err)
	}

	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	if err = r.manipulateObj(ctx, grafanaDeployer, mapper, grafanaObjects, mgh,
		condition.SetConditionDatabaseInit, log); err != nil {
		return err
	}

	log.Info("Grafana initialized")
	if err := condition.SetConditionGrafanaInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE); err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}
