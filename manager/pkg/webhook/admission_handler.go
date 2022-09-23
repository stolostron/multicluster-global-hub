// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var log = logf.Log.WithName("admission-handler")

// AdmissionHandler is to handle the admission webhook for placementrule and placement
type AdmissionHandler struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (a *AdmissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.Info("admission webhook is called", "name", req.Name, "namespace", req.Namespace, "kind", req.Kind.Kind, "operation", req.Operation)
	return admission.PatchResponseFromRaw(req.Object.Raw, req.Object.Raw)
}

// AdmissionHandler implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (a *AdmissionHandler) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
