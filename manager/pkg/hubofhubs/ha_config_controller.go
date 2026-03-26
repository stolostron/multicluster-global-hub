package hubofhubs

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
	"k8s.io/apimachinery/pkg/types"
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
	localClusterNamespace   = "local-cluster"
	mceNamespace            = "multicluster-engine"
	haConfigMSAPrefix       = "ha-config-"
	haConfigOwnerLabel      = "ha-config"
	haBootstrapPrefix       = "bootstrap-ha-"
	haConfigRequeueInterval = 5 * time.Second
	haConfigEventExpiry     = 10 * time.Minute
)

var haConfigLog = logger.DefaultZapLogger()

type HAConfigController struct {
	client.Client
	transport.Producer
	Scheme *runtime.Scheme
}

var haConfigCtrl *HAConfigController

func AddHAConfigToManager(mgr ctrl.Manager, producer transport.Producer) error {
	if haConfigCtrl != nil {
		return nil
	}
	c := &HAConfigController{
		Client:   mgr.GetClient(),
		Producer: producer,
		Scheme:   mgr.GetScheme(),
	}
	if err := c.SetupWithManager(mgr); err != nil {
		return err
	}
	haConfigCtrl = c
	return nil
}

func (h *HAConfigController) SetupWithManager(mgr ctrl.Manager) error {
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
				if !strings.HasPrefix(name, haConfigMSAPrefix) {
					return nil
				}
				activeHubName := strings.TrimPrefix(name, haConfigMSAPrefix)
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{Name: activeHubName}},
				}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true" &&
						e.Object.GetNamespace() == localClusterNamespace
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true" &&
						e.ObjectNew.GetNamespace() == localClusterNamespace &&
						e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			}),
		).
		Complete(h)
}

func (h *HAConfigController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	haConfigLog.Debugf("reconcile HA config for %v", req)

	mc := &clusterv1.ManagedCluster{}
	if err := h.Get(ctx, types.NamespacedName{Name: req.Name}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, h.cleanup(ctx, req.Name)
		}
		return ctrl.Result{}, err
	}

	if mc.DeletionTimestamp != nil || mc.Labels[constants.GHHubRoleLabelKey] != constants.GHHubRoleActive {
		return ctrl.Result{}, h.cleanup(ctx, req.Name)
	}

	activeHubName := mc.Name

	if err := h.ensureManagedServiceAccount(ctx, activeHubName); err != nil {
		return ctrl.Result{}, err
	}

	bootstrapSecret, err := h.generateBootstrapSecret(ctx, activeHubName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			haConfigLog.Infof("MSA token secret not yet available for %s, requeuing", activeHubName)
			return ctrl.Result{RequeueAfter: haConfigRequeueInterval}, nil
		}
		return ctrl.Result{}, err
	}

	if err := h.sendHAConfigEvent(ctx, activeHubName, bootstrapSecret); err != nil {
		return ctrl.Result{}, err
	}

	haConfigLog.Infof("HA config event sent for active hub %s", activeHubName)
	return ctrl.Result{}, nil
}

func (h *HAConfigController) ensureManagedServiceAccount(ctx context.Context, activeHubName string) error {
	msaName := haConfigMSAPrefix + activeHubName
	desiredMSA := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      msaName,
			Namespace: localClusterNamespace,
			Labels: map[string]string{
				"owner": haConfigOwnerLabel,
			},
		},
		Spec: v1beta1.ManagedServiceAccountSpec{
			Rotation: v1beta1.ManagedServiceAccountRotation{
				Enabled: true,
				Validity: metav1.Duration{
					Duration: 24 * time.Hour,
				},
			},
		},
	}

	existingMSA := &v1beta1.ManagedServiceAccount{}
	err := h.Get(ctx, types.NamespacedName{
		Name:      msaName,
		Namespace: localClusterNamespace,
	}, existingMSA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return h.Create(ctx, desiredMSA)
		}
		return err
	}
	return nil
}

func (h *HAConfigController) generateBootstrapSecret(ctx context.Context,
	activeHubName string,
) (*corev1.Secret, error) {
	msaSecret := &corev1.Secret{}
	if err := h.Get(ctx, types.NamespacedName{
		Name:      haConfigMSAPrefix + activeHubName,
		Namespace: localClusterNamespace,
	}, msaSecret); err != nil {
		return nil, err
	}

	managedCluster := &clusterv1.ManagedCluster{}
	if err := h.Get(ctx, types.NamespacedName{
		Name: localClusterNamespace,
	}, managedCluster); err != nil {
		return nil, err
	}

	if len(managedCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return nil, fmt.Errorf("no ManagedClusterClientConfigs found for %s", localClusterNamespace)
	}

	config := clientcmdapi.NewConfig()
	config.Clusters[localClusterNamespace] = &clientcmdapi.Cluster{
		Server:                   managedCluster.Spec.ManagedClusterClientConfigs[0].URL,
		CertificateAuthorityData: msaSecret.Data["ca.crt"],
	}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: string(msaSecret.Data["token"]),
	}
	config.Contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  localClusterNamespace,
		AuthInfo: "user",
	}
	config.CurrentContext = "default-context"

	kubeconfigBytes, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}

	bootstrapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      haBootstrapPrefix + localClusterNamespace,
			Namespace: mceNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}
	return bootstrapSecret, nil
}

func (h *HAConfigController) sendHAConfigEvent(ctx context.Context,
	activeHubName string, bootstrapSecret *corev1.Secret,
) error {
	bundle := &haconfigbundle.HAConfigBundle{
		ActiveHubName:   activeHubName,
		StandbyHubName:  localClusterNamespace,
		BootstrapSecret: bootstrapSecret,
	}

	payloadBytes, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal HA config bundle: %w", err)
	}

	evt := utils.ToCloudEvent(constants.HAConfigMsgKey,
		constants.CloudEventGlobalHubClusterName, activeHubName, payloadBytes)
	evt.SetExtension(constants.CloudEventExtensionKeyExpireTime,
		time.Now().Add(haConfigEventExpiry).Format(time.RFC3339))
	if err := h.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to send HA config event to %s: %w", activeHubName, err)
	}
	return nil
}

func (h *HAConfigController) cleanup(ctx context.Context, activeHubName string) error {
	msaName := haConfigMSAPrefix + activeHubName
	msa := &v1beta1.ManagedServiceAccount{}
	err := h.Get(ctx, types.NamespacedName{
		Name:      msaName,
		Namespace: localClusterNamespace,
	}, msa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	haConfigLog.Infof("deleting MSA %s/%s for HA config cleanup", localClusterNamespace, msaName)
	return h.Delete(ctx, msa)
}
