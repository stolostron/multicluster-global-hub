// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

var log = logf.Log.WithName("admission-handler")

type admissionHandler struct {
	client  client.Client
	decoder *admission.Decoder
}

func (a *admissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.V(2).Info("admission webhook is called", "name", req.Name, "namespace",
		req.Namespace, "kind", req.Kind.Kind, "operation", req.Operation)

	if req.Kind.Kind == "Placement" {
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
	} else if req.Kind.Kind == "PlacementRule" {
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
	}

	return admission.Allowed("")
}

// AdmissionHandler implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *admissionHandler) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
