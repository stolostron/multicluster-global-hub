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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewAdmissionHandler is to handle the admission webhook for placementrule and placement
func NewAdmissionHandler(c client.Client, s *runtime.Scheme) admission.Handler {
	return &admissionHandler{
		client:  c,
		decoder: admission.NewDecoder(s),
	}
}

type admissionHandler struct {
	client  client.Client
	decoder admission.Decoder
}

func (a *admissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	klog.V(2).Infof("admission webhook is called, name:%v, namespace:%v, kind:%v, operation:%v", req.Name,
		req.Namespace, req.Kind.Kind, req.Operation)
	switch req.Kind.Kind {
	case "Placement":
		placement := &clusterv1beta1.Placement{}
		err := a.decoder.Decode(req, placement)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		// don't schedule the policy/application for global hub resources
		if _, found := placement.Labels[constants.GlobalHubGlobalResourceLabel]; found {
			if placement.Annotations == nil {
				placement.Annotations = map[string]string{}
			}
			placement.Annotations[clusterv1beta1.PlacementDisableAnnotation] = "true"

			marshaledPlacement, err := json.Marshal(placement)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPlacement)
		}
		return admission.Allowed("")
	case "PlacementRule":
		placementrule := &placementrulesv1.PlacementRule{}
		err := a.decoder.Decode(req, placementrule)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if _, found := placementrule.Labels[constants.GlobalHubGlobalResourceLabel]; found {
			placementrule.Spec.SchedulerName = constants.GlobalHubSchedulerName

			marshaledPlacementRule, err := json.Marshal(placementrule)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPlacementRule)
		}
		return admission.Allowed("")
	case "ManagedCluster":
		cluster := &clusterv1.ManagedCluster{}
		err := a.decoder.Decode(req, cluster)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if cluster.Name == constants.LocalClusterName {
			return admission.Allowed("")
		}

		changed := setHostedAnnotations(cluster)
		if !changed {
			return admission.Allowed("")
		}

		klog.Infof("Add hosted annotation for managedcluster: %v", cluster.Name)

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

		// only handle hosted clusters
		isHosted, err := isInHostedCluster(ctx, a.client, klusterletaddonconfig.Namespace)
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
		klog.Infof("Disable addons in cluster :%v", klusterletaddonconfig.Namespace)

		marshaledKlusterletAddon, err := json.Marshal(klusterletaddonconfig)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledKlusterletAddon)
	default:
		return admission.Allowed("")
	}
}

// isInHostedCluster check if the cluster has hosted annotations
func isInHostedCluster(ctx context.Context, client client.Client, mcName string) (bool, error) {
	mc := &clusterv1.ManagedCluster{}
	err := client.Get(ctx, types.NamespacedName{Name: mcName}, mc)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		}
		errMsg := fmt.Errorf("failed to get managedcluster, err:%v", err)
		klog.Errorf(errMsg.Error())
		return false, errMsg
	}

	if (mc.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted) &&
		(mc.Annotations[constants.AnnotationClusterHostingClusterName] == constants.LocalClusterName) {
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
func setHostedAnnotations(cluster *clusterv1.ManagedCluster) bool {
	if (cluster.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted) &&
		(cluster.Annotations[constants.AnnotationClusterHostingClusterName] == constants.LocalClusterName) {
		return false
	}
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	cluster.Annotations[constants.AnnotationClusterDeployMode] = constants.ClusterDeployModeHosted
	cluster.Annotations[constants.AnnotationClusterHostingClusterName] = constants.LocalClusterName
	return true
}

// AdmissionHandler implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *admissionHandler) InjectDecoder(d admission.Decoder) error {
	a.decoder = d
	return nil
}
