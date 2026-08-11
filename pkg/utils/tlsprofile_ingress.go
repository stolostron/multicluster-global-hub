/*
Copyright Contributors to the Open Cluster Management project.
*/

package utils

import (
	"context"
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultIngressControllerName       = "default"
	defaultIngressControllerNamespace  = "openshift-ingress-operator"
	errMsgFailedToGetIngressController = "failed to get IngressController %s/%s: %w"
)

// FetchIngressControllerTLSProfileSpec returns the effective TLS profile from the
// default OpenShift IngressController (status.tlsProfile).
//
// When status.tlsProfile is unset, it falls back to the IngressController
// spec.tlsSecurityProfile (resolved via resolveSpec), then to the cluster
// APIServer profile. A nil/empty result defaults to Intermediate.
//
// This is the TLS source for OpenShift Route edge/reencrypt client-facing hops
// (HAProxy), as opposed to APIServer which is the source for first-party Go servers.
func FetchIngressControllerTLSProfileSpec(
	ctx context.Context,
	c client.Client,
) (*configv1.TLSProfileSpec, error) {
	ic := &operatorv1.IngressController{}
	key := client.ObjectKey{
		Name:      defaultIngressControllerName,
		Namespace: defaultIngressControllerNamespace,
	}
	if err := c.Get(ctx, key, ic); err != nil {
		if apierrors.IsNotFound(err) {
			// Non-OpenShift or Ingress Operator not installed: fall back to APIServer.
			return fetchAPIServerTLSProfileSpec(ctx, c)
		}
		return nil, fmt.Errorf(errMsgFailedToGetIngressController,
			defaultIngressControllerNamespace, defaultIngressControllerName, err)
	}

	if ic.Status.TLSProfile != nil {
		return ic.Status.TLSProfile.DeepCopy(), nil
	}

	if ic.Spec.TLSSecurityProfile != nil {
		return resolveSpec(ic.Spec.TLSSecurityProfile)
	}

	return fetchAPIServerTLSProfileSpec(ctx, c)
}

func fetchAPIServerTLSProfileSpec(
	ctx context.Context,
	c client.Client,
) (*configv1.TLSProfileSpec, error) {
	apiserver := &configv1.APIServer{}
	if err := c.Get(ctx, client.ObjectKey{Name: "cluster"}, apiserver); err != nil {
		if apierrors.IsNotFound(err) {
			return resolveSpec(nil)
		}
		return nil, fmt.Errorf(errMsgFailedToGetAPIServer, err)
	}
	return resolveSpec(apiserver.Spec.TLSSecurityProfile)
}

// BuildTLSConfigFromIngressProfileSpec builds a tls.Config from an IngressController
// status.tlsProfile (or any resolved TLSProfileSpec).
func BuildTLSConfigFromIngressProfileSpec(spec *configv1.TLSProfileSpec) (*tls.Config, error) {
	if spec == nil {
		resolved, err := resolveSpec(nil)
		if err != nil {
			return nil, err
		}
		spec = resolved
	}

	minVer, err := parseTLSVersion(string(spec.MinTLSVersion))
	if err != nil {
		return nil, fmt.Errorf("invalid MinTLSVersion: %w", err)
	}

	// #nosec G402 -- MinVersion comes from the cluster Ingress TLS profile.
	cfg := &tls.Config{MinVersion: minVer}
	if minVer < tls.VersionTLS13 && len(spec.Ciphers) > 0 {
		suites := mapCipherSuites(spec.Ciphers)
		if len(suites) == 0 {
			return nil, fmt.Errorf(
				"no valid cipher suites found for Ingress TLS profile (all %d cipher(s) unsupported by Go)",
				len(spec.Ciphers),
			)
		}
		cfg.CipherSuites = suites
	}
	return cfg, nil
}

// GetTLSConfigFromIngressController fetches the default IngressController TLS profile
// and builds a tls.Config. Prefer this for surfaces that terminate behind OpenShift Routes.
func GetTLSConfigFromIngressController(ctx context.Context, c client.Client) (*tls.Config, error) {
	spec, err := FetchIngressControllerTLSProfileSpec(ctx, c)
	if err != nil {
		return nil, err
	}
	return BuildTLSConfigFromIngressProfileSpec(spec)
}
