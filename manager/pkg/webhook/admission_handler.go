// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

// NewAdmissionHandler is to handle the admission webhook for placementrule and placement
func NewAdmissionHandler(s *runtime.Scheme) admission.Handler {
	return &admissionHandler{
		decoder: admission.NewDecoder(s),
	}
}

type admissionHandler struct {
	decoder admission.Decoder
}

func (a *admissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.Infof("admission webhook is called, name:%v, namespace:%v, kind:%v, operation:%v", req.Name,
		req.Namespace, req.Kind.Kind, req.Operation)
	switch req.Kind.Kind {
	case "Placement":
		placement := &clusterv1beta1.Placement{}
		err := a.decoder.Decode(req, placement)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		// Global resource feature removed - no special processing needed
		return admission.Allowed("")
	case "PlacementRule":
		placementrule := &placementrulesv1.PlacementRule{}
		err := a.decoder.Decode(req, placementrule)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		// Global resource feature removed - no special processing needed
		return admission.Allowed("")
	default:
		return admission.Allowed("")
	}
}

// AdmissionHandler implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *admissionHandler) InjectDecoder(d admission.Decoder) error {
	a.decoder = d
	return nil
}
