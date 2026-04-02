package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	haconfigbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/haconfig"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	mceNamespace    = "multicluster-engine"
	msaPrefix       = "ha-config-"
	ownerLabel      = "ha-config"
	bootstrapPrefix = "bootstrap-ha-"
	requeueInterval = 5 * time.Second
	eventExpiry     = 10 * time.Minute
)

var log = logger.DefaultZapLogger()

type ConfigController struct {
	client.Client
	transport.Producer
	Scheme *runtime.Scheme
}

var configCtrl *ConfigController

func AddToManager(mgr ctrl.Manager, producer transport.Producer) error {
	if configCtrl != nil {
		return nil
	}
	c := &ConfigController{
		Client:   mgr.GetClient(),
		Producer: producer,
		Scheme:   mgr.GetScheme(),
	}
	if err := c.SetupWithManager(mgr); err != nil {
		return err
	}
	configCtrl = c
	return nil
}

func (c *ConfigController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("ha-config-ctrl").
		Watches(&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetLabels()[constants.GHHubRoleLabelKey] == constants.GHHubRoleActive
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldVal := e.ObjectOld.GetLabels()[constants.GHHubRoleLabelKey]
					newVal := e.ObjectNew.GetLabels()[constants.GHHubRoleLabelKey]
					return oldVal != newVal
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetLabels()[constants.GHHubRoleLabelKey] == constants.GHHubRoleActive
				},
			}),
		).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				name := obj.GetName()
				if !strings.HasPrefix(name, msaPrefix) {
					return nil
				}
				activeHubName := strings.TrimPrefix(name, msaPrefix)
				return []reconcile.Request{
					{NamespacedName: client.ObjectKey{Name: activeHubName}},
				}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true" &&
						strings.HasPrefix(e.Object.GetName(), msaPrefix)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true" &&
						strings.HasPrefix(e.ObjectNew.GetName(), msaPrefix) &&
						e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			}),
		).
		Complete(c)
}

func (c *ConfigController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile HA config for %v", req)

	// Step 1: find local-cluster by label, HA requires it as the standby hub
	localCluster, err := c.getLocalCluster(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if localCluster == nil {
		log.Infof("local-cluster not found, HA feature requires a cluster with label %s=true",
			constants.LocalClusterName)
		return ctrl.Result{}, nil
	}
	localClusterName := localCluster.Name

	// Step 2: check if the managed cluster is an active hub
	mc := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: req.Name}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, c.removeActiveHubResources(ctx, localClusterName, req.Name)
		}
		return ctrl.Result{}, err
	}

	if mc.DeletionTimestamp != nil || mc.Labels[constants.GHHubRoleLabelKey] != constants.GHHubRoleActive {
		return ctrl.Result{}, c.removeActiveHubResources(ctx, localClusterName, req.Name)
	}

	activeHubName := mc.Name

	if err := c.ensureMSA(ctx, localClusterName, activeHubName); err != nil {
		return ctrl.Result{}, err
	}

	bootstrapSecret, err := c.generateBootstrapSecret(ctx, localCluster, activeHubName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("MSA token secret not yet available for %s, requeuing", activeHubName)
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
		return ctrl.Result{}, err
	}

	if err := c.sendHAConfig(ctx, localClusterName, activeHubName, bootstrapSecret, eventExpiry); err != nil {
		return ctrl.Result{}, err
	}

	log.Infof("HA config sent: source=%s, activeHub=%s, expiry=%s", localClusterName, activeHubName, eventExpiry)
	return ctrl.Result{}, nil
}

// getLocalCluster finds the local cluster by the label "local-cluster=true".
// Returns nil if no local cluster is found.
func (c *ConfigController) getLocalCluster(ctx context.Context) (*clusterv1.ManagedCluster, error) {
	clusterList := &clusterv1.ManagedClusterList{}
	if err := c.List(ctx, clusterList, client.MatchingLabels{
		constants.LocalClusterName: "true",
	}); err != nil {
		return nil, err
	}
	if len(clusterList.Items) == 0 {
		return nil, nil
	}
	return &clusterList.Items[0], nil
}

func (c *ConfigController) ensureMSA(ctx context.Context, localClusterName, activeHubName string) error {
	msaName := msaPrefix + activeHubName
	desired := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      msaName,
			Namespace: localClusterName,
			Labels: map[string]string{
				"owner": ownerLabel,
			},
		},
		Spec: v1beta1.ManagedServiceAccountSpec{
			Rotation: v1beta1.ManagedServiceAccountRotation{
				Enabled: true,
				Validity: metav1.Duration{
					Duration: 365 * 24 * time.Hour,
				},
			},
		},
	}

	existing := &v1beta1.ManagedServiceAccount{}
	err := c.Get(ctx, client.ObjectKey{
		Name:      msaName,
		Namespace: localClusterName,
	}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return c.Create(ctx, desired)
		}
		return err
	}
	return nil
}

func (c *ConfigController) generateBootstrapSecret(ctx context.Context,
	localCluster *clusterv1.ManagedCluster, activeHubName string,
) (*corev1.Secret, error) {
	localClusterName := localCluster.Name

	msaSecret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      msaPrefix + activeHubName,
		Namespace: localClusterName,
	}, msaSecret); err != nil {
		return nil, err
	}

	if len(msaSecret.Data["ca.crt"]) == 0 || len(msaSecret.Data["token"]) == 0 {
		return nil, fmt.Errorf("MSA secret %s/%s is missing required data (ca.crt or token)",
			localClusterName, msaPrefix+activeHubName)
	}

	if len(localCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return nil, fmt.Errorf("no ManagedClusterClientConfigs found for %s", localClusterName)
	}

	config := clientcmdapi.NewConfig()
	config.Clusters[localClusterName] = &clientcmdapi.Cluster{
		Server:                   localCluster.Spec.ManagedClusterClientConfigs[0].URL,
		CertificateAuthorityData: msaSecret.Data["ca.crt"],
	}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: string(msaSecret.Data["token"]),
	}
	config.Contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  localClusterName,
		AuthInfo: "user",
	}
	config.CurrentContext = "default-context"

	kubeconfigBytes, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapPrefix + localClusterName,
			Namespace: mceNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}, nil
}

func (c *ConfigController) sendHAConfig(ctx context.Context,
	sourceHub, activeHub string, bootstrapSecret *corev1.Secret, expiry time.Duration,
) error {
	bundle := &haconfigbundle.HAConfigBundle{
		BootstrapSecret: bootstrapSecret,
	}

	payloadBytes, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal HA config bundle: %w", err)
	}

	evt := utils.ToCloudEvent(constants.HAConfigMsgKey, sourceHub, activeHub, payloadBytes)
	evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
		time.Now().Add(expiry).Format(time.RFC3339))
	if err := c.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to send HA config to activeHub=%s: %w", activeHub, err)
	}
	return nil
}

func (c *ConfigController) removeActiveHubResources(ctx context.Context,
	localClusterName, activeHubName string,
) error {
	msaName := msaPrefix + activeHubName
	msa := &v1beta1.ManagedServiceAccount{}
	err := c.Get(ctx, client.ObjectKey{
		Name:      msaName,
		Namespace: localClusterName,
	}, msa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	log.Infof("deleting MSA %s/%s for HA config cleanup", localClusterName, msaName)
	return c.Delete(ctx, msa)
}
