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
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
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

		a.localClusterName, err = getLocalClusterName(ctx, a.client)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// Add the annotation 'klusterlet-deploy-mode=hosted' only if the feature gate enabled -> cluster(klusterlet)
		if !config.GetImportClusterInHosted() {
			return admission.Allowed("")
		}

		changed := a.setHostedAnnotations(cluster)
		if !changed {
			return admission.Allowed("")
		}

		log.Infof("Add hosted annotation for managedcluster: %v", cluster.Name)

		marshaledCluster, err := json.Marshal(cluster)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledCluster)

	case "KlusterletAddonConfig":
		klusterletaddonconfig := &addonv1.KlusterletAddonConfig{}
		err := a.decoder.Decode(req, klusterletaddonconfig)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if klusterletaddonconfig.Spec.ClusterLabels[constants.LocalClusterName] == "true" {
			return admission.Allowed("")
		}

		a.localClusterName, err = getLocalClusterName(ctx, a.client)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// only handle hosted clusters
		isHosted, err := a.isInHostedCluster(ctx, a.client, klusterletaddonconfig.Namespace)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if !isHosted {
			return admission.Allowed("")
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
	default:
		return admission.Allowed("")
	}
}

func getLocalClusterName(ctx context.Context, client client.Client) (string, error) {
	mcList := &clusterv1.ManagedClusterList{}
	err := client.List(ctx, mcList)
	if err != nil {
		return "", fmt.Errorf("there is no clusters, the local clusters should enable,err:%v", err.Error())
	}
	for _, mc := range mcList.Items {
		if mc.Labels[constants.LocalClusterName] == "true" {
			return mc.Name, nil
		}
	}
	return "", fmt.Errorf("the local clusters should be enabled")
}

// isInHostedCluster check if the cluster has hosted annotations
func (a *admissionHandler) isInHostedCluster(ctx context.Context, client client.Client, mcName string) (bool, error) {
	mc := &clusterv1.ManagedCluster{}
	err := client.Get(ctx, types.NamespacedName{Name: mcName}, mc)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		}
		errMsg := fmt.Errorf("failed to get managedcluster, err:%v", err)
		log.Errorf(errMsg.Error())
		return false, errMsg
	}

	if (mc.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted) &&
		(mc.Annotations[constants.AnnotationClusterHostingClusterName] == a.localClusterName) {
		return true, nil
	}
	return false, nil
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
