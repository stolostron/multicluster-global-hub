// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// NewAdmissionHandler is to handle the admission webhook for placementrule and placement
func NewAdmissionHandler(c client.Client, s *runtime.Scheme) admission.Handler {
	return &admissionHandler{
		client:  c,
		decoder: admission.NewDecoder(s),
	}
}

type admissionHandler struct {
	client           client.Client
	decoder          admission.Decoder
	localClusterName string
}

func (a *admissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.Debugf("admission webhook is called, name:%v, namespace:%v, kind:%v, operation:%v", req.Name,
		req.Namespace, req.Kind.Kind, req.Operation)

	switch req.Kind.Kind {
	case "ManagedCluster":
		return a.handleManagedCluster(ctx, req)
	case "KlusterletAddonConfig":
		return a.handleKlusterletAddonConfig(ctx, req)
	default:
		return admission.Allowed("")
	}
}

// handleManagedCluster handles the admission request for ManagedCluster
// It checks if the cluster is labeled for hosted mode, and if so, adds the necessary annotations.
// If not labeled, it allows the request to proceed as normal.
// If the KlusterletAddonConfig exists, should disable the addons for hosted mode.
func (a *admissionHandler) handleManagedCluster(ctx context.Context, req admission.Request) admission.Response {
	cluster := &clusterv1.ManagedCluster{}
	err := a.decoder.Decode(req, cluster)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if cluster.Labels[constants.LocalClusterName] == "true" {
		return admission.Allowed("")
	}

	// If cluster already imported, skip it
	if meta.IsStatusConditionTrue(cluster.Status.Conditions, constants.ManagedClusterImportSucceeded) {
		return admission.Allowed("")
	}

	// Check the importing label
	deployMode, ok := cluster.Labels[constants.GHDeployModeLabelKey]
	if !ok {
		log.Infof("The cluster %s does not have the label, importing as a managed cluster", cluster.Name)
		return admission.Allowed(fmt.Sprintf("The cluster %s does not have the label %s, importing as a managed cluster",
			cluster.Name, constants.GHDeployModeLabelKey))
	}

	if deployMode != constants.GHDeployModeHosted && deployMode != constants.GHDeployModeDefault {
		return admission.Denied(fmt.Sprintf("The cluster %s with invalid label %s=%s, only support %s and %s",
			cluster.Name, constants.GHDeployModeLabelKey, deployMode, constants.GHDeployModeHosted,
			constants.GHDeployModeDefault))
	}

	if deployMode == constants.GHDeployModeDefault {
		return admission.Allowed(fmt.Sprintf("The cluster %s with label %s=%s, importing the managed hub in default mode",
			cluster.Name, constants.GHDeployModeLabelKey, deployMode))
	}

	// If hosted mode is enabled, the local cluster must also be enabled, since the klusterlet-agent of the hosted
	// cluster must be reconciled by the klusterlet operator(required) running in the current (global hub) cluster.
	if deployMode == constants.GHDeployModeHosted {
		log.Infof("The cluster %s with label %s=%s, importing the managed hub in hosted mode",
			cluster.Name, constants.GHDeployModeLabelKey, deployMode)

		a.localClusterName, err = getLocalClusterName(ctx, a.client)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// get klusterletaddonconfig, and disable addons if exists
		kac := &addonv1.KlusterletAddonConfig{}
		err = a.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Name}, kac)
		if err != nil {
			if !errors.IsNotFound(err) {
				log.Errorf("failed to get klusterletaddonconfig, err:%v", err)
			}
		} else {
			if disableAddons(kac) {
				if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					return a.client.Update(ctx, kac)
				}); err != nil {
					log.Errorf("failed to update klusterletaddonconfig, err:%v", err)
				}
			}
		}

		// set hosted annotations
		changed := a.setHostedAnnotations(cluster)
		if !changed {
			return admission.Allowed("")
		}

		log.Infof("Add hosted annotation into managedcluster: %v", cluster.Name)
		marshaledCluster, err := json.Marshal(cluster)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledCluster)
	}
	return admission.Allowed("")
}

// handleKlusterletAddonConfig handles the admission request for KlusterletAddonConfig
// It checks if the corresponding ManagedCluster exists and is in hosted mode.
// If so, it disables the addons in the KlusterletAddonConfig.
func (a *admissionHandler) handleKlusterletAddonConfig(ctx context.Context, req admission.Request) admission.Response {

	klusterletaddonconfig := &addonv1.KlusterletAddonConfig{}
	err := a.decoder.Decode(req, klusterletaddonconfig)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Check if the corresponding ManagedCluster exists
	mc := &clusterv1.ManagedCluster{}
	err = a.client.Get(ctx, types.NamespacedName{Name: klusterletaddonconfig.Namespace}, mc)
	if err != nil {
		if errors.IsNotFound(err) {
			return admission.Allowed("")
		}
		// do not block the request if error occurs
		log.Errorf("failed to get managedcluster, err:%v", err)
	}

	// only handle hosted clusters
	if (mc.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted) &&
		(mc.Annotations[constants.AnnotationClusterHostingClusterName] == a.localClusterName) {

		log.Infof("handling klusterletaddonconfig for hosted cluster: %s", klusterletaddonconfig.Name)
		a.localClusterName, err = getLocalClusterName(ctx, a.client)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		changed := disableAddons(klusterletaddonconfig)
		if !changed {
			return admission.Allowed("")
		}
		log.Infof("Disable addons in cluster :%v", klusterletaddonconfig.Namespace)

		marshaledKlusterletAddon, err := json.Marshal(klusterletaddonconfig)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledKlusterletAddon)
	}
	return admission.Allowed("")
}

// getLocalClusterName gets the local cluster name of the current cluster,
func getLocalClusterName(ctx context.Context, client client.Client) (string, error) {
	mcList := &clusterv1.ManagedClusterList{}
	err := client.List(ctx, mcList)
	if err != nil {
		return "", fmt.Errorf("the local cluster must be enabled, err: %v", err.Error())
	}
	for _, mc := range mcList.Items {
		if mc.Labels[constants.LocalClusterName] == "true" {
			return mc.Name, nil
		}
	}
	return "", fmt.Errorf("the local cluster must be enabled")
}

// disableAddons disable addons in klusterletaddonconfig, return true if changed
func disableAddons(klusterletaddonconfig *addonv1.KlusterletAddonConfig) bool {
	changed := false
	if klusterletaddonconfig.Spec.ApplicationManagerConfig.Enabled {
		klusterletaddonconfig.Spec.ApplicationManagerConfig.Enabled = false
		changed = true
	}
	if klusterletaddonconfig.Spec.PolicyController.Enabled {
		klusterletaddonconfig.Spec.PolicyController.Enabled = false
		changed = true
	}
	if klusterletaddonconfig.Spec.CertPolicyControllerConfig.Enabled {
		klusterletaddonconfig.Spec.CertPolicyControllerConfig.Enabled = false
		changed = true
	}

	return changed
}

// setHostedAnnotations set hosted annotation for cluster, and return true if changed
func (a *admissionHandler) setHostedAnnotations(cluster *clusterv1.ManagedCluster) bool {
	if (cluster.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted) &&
		(cluster.Annotations[constants.AnnotationClusterHostingClusterName] == a.localClusterName) {
		return false
	}
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	cluster.Annotations[constants.AnnotationClusterDeployMode] = constants.ClusterDeployModeHosted
	cluster.Annotations[constants.AnnotationClusterHostingClusterName] = a.localClusterName
	return true
}

// AdmissionHandler implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *admissionHandler) InjectDecoder(d admission.Decoder) error {
	a.decoder = d
	return nil
}
