package utils

import (
	"context"
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	configclientset "github.com/openshift/client-go/config/clientset/versioned"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetTLSConfigFromAPIServer fetches the TLS profile from the OpenShift APIServer
// and builds a tls.Config based on cluster-wide TLS security profile.
// This is the recommended approach for OpenShift components.
func GetTLSConfigFromAPIServer(ctx context.Context, restConfig *rest.Config) (*tls.Config, error) {
	configClient, err := configclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	apiserver, err := configClient.ConfigV1().APIServers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer config: %w", err)
	}

	profile := apiserver.Spec.TLSSecurityProfile
	if profile == nil {
		// Use Intermediate profile as default when no profile is specified
		profile = &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
	}

	return BuildTLSConfig(profile)
}

// GetTLSConfigFromClient fetches the TLS profile using a controller-runtime client.
// This is useful when you already have a controller-runtime client available.
func GetTLSConfigFromClient(ctx context.Context, c client.Client) (*tls.Config, error) {
	apiserver := &configv1.APIServer{}
	if err := c.Get(ctx, client.ObjectKey{Name: "cluster"}, apiserver); err != nil {
		return nil, fmt.Errorf("failed to get APIServer config: %w", err)
	}

	profile := apiserver.Spec.TLSSecurityProfile
	if profile == nil {
		// Use Intermediate profile as default when no profile is specified
		profile = &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
	}

	return BuildTLSConfig(profile)
}

// BuildTLSConfig builds a tls.Config from an OpenShift TLS security profile.
func BuildTLSConfig(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	spec, err := resolveSpec(profile)
	if err != nil {
		return nil, err
	}

	minVer, err := parseTLSVersion(string(spec.MinTLSVersion))
	if err != nil {
		return nil, fmt.Errorf("invalid MinTLSVersion: %w", err)
	}

	cfg := &tls.Config{MinVersion: minVer}

	// TLS 1.3 cipher suites are not configurable in Go (golang/go#29349).
	// Only set CipherSuites for TLS 1.2 and below.
	if minVer < tls.VersionTLS13 {
		suites := mapCipherSuites(spec.Ciphers)
		if len(suites) == 0 {
			return nil, fmt.Errorf("no valid cipher suites found for TLS profile")
		}
		cfg.CipherSuites = suites
	}

	return cfg, nil
}

// BuildTLSConfigFunc returns a function that can be used to configure tls.Config.
// This is useful for controller-runtime's metricsserver.Options.TLSOpts.
func BuildTLSConfigFunc(profile *configv1.TLSSecurityProfile) (func(*tls.Config), error) {
	spec, err := resolveSpec(profile)
	if err != nil {
		return nil, err
	}

	minVer, err := parseTLSVersion(string(spec.MinTLSVersion))
	if err != nil {
		return nil, fmt.Errorf("invalid MinTLSVersion: %w", err)
	}

	// TLS 1.3 cipher suites are not configurable in Go, so validate cipher suites
	// early for TLS < 1.3 to fail fast if the profile specifies unsupported ciphers
	var suites []uint16
	if minVer < tls.VersionTLS13 {
		suites = mapCipherSuites(spec.Ciphers)
		if len(suites) == 0 && len(spec.Ciphers) > 0 {
			return nil, fmt.Errorf("no valid cipher suites found for TLS profile (all %d cipher(s) unsupported by Go)", len(spec.Ciphers))
		}
	}

	return func(cfg *tls.Config) {
		cfg.MinVersion = minVer
		// Always set CipherSuites to ensure consistent behavior:
		// - TLS < 1.3: use specific suites or nil for Go defaults
		// - TLS >= 1.3: nil (cipher suites not configurable in Go)
		cfg.CipherSuites = suites
	}, nil
}

// resolveSpec resolves the TLSProfileSpec from a TLSSecurityProfile.
// It handles both built-in profiles (Old, Intermediate, Modern) and custom profiles.
func resolveSpec(profile *configv1.TLSSecurityProfile) (*configv1.TLSProfileSpec, error) {
	// Handle nil profile by defaulting to Intermediate
	if profile == nil {
		spec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		if spec == nil {
			return nil, fmt.Errorf("default Intermediate TLS profile not found in configv1.TLSProfiles")
		}
		return spec, nil
	}

	switch profile.Type {
	case configv1.TLSProfileOldType,
		configv1.TLSProfileIntermediateType,
		configv1.TLSProfileModernType:
		spec := configv1.TLSProfiles[profile.Type]
		if spec == nil {
			return nil, fmt.Errorf("TLS profile %s not found in configv1.TLSProfiles", profile.Type)
		}
		return spec, nil
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return nil, fmt.Errorf("custom TLS profile specified but Custom is nil")
		}
		return &profile.Custom.TLSProfileSpec, nil
	default:
		// Return error for unknown/unsupported profile types instead of silently defaulting
		return nil, fmt.Errorf("unsupported TLS profile type %q", profile.Type)
	}
}

// parseTLSVersion converts an OpenShift TLS version string to a Go TLS version constant.
func parseTLSVersion(v string) (uint16, error) {
	versions := map[string]uint16{
		"VersionTLS10": tls.VersionTLS10,
		"VersionTLS11": tls.VersionTLS11,
		"VersionTLS12": tls.VersionTLS12,
		"VersionTLS13": tls.VersionTLS13,
	}
	if ver, ok := versions[v]; ok {
		return ver, nil
	}
	return 0, fmt.Errorf("unknown TLS version: %s", v)
}

// mapCipherSuites converts OpenSSL-style cipher names (used in OpenShift TLS
// profiles) to Go crypto/tls constants. Ciphers without a Go constant (e.g.
// DHE-RSA-*, ECDHE-RSA-AES256-SHA384, AES256-SHA256) are silently skipped —
// Go's crypto/tls does not support them.
func mapCipherSuites(names []string) []uint16 {
	m := map[string]uint16{
		"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"ECDHE-RSA-AES128-SHA256":       tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		"ECDHE-ECDSA-AES128-SHA256":     tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		"ECDHE-RSA-AES128-SHA":          tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"ECDHE-ECDSA-AES128-SHA":        tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"ECDHE-RSA-AES256-SHA":          tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		"ECDHE-ECDSA-AES256-SHA":        tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"AES128-GCM-SHA256":             tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"AES256-GCM-SHA384":             tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		"AES128-SHA256":                 tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		"AES128-SHA":                    tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"AES256-SHA":                    tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"DES-CBC3-SHA":                  tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	}

	out := make([]uint16, 0, len(names))
	for _, name := range names {
		if id, ok := m[name]; ok {
			out = append(out, id)
		}
	}
	return out
}

// GetOpenShiftConfigClient creates a versioned config client for accessing OpenShift config resources.
func GetOpenShiftConfigClient(restConfig *rest.Config) (configv1client.ConfigV1Interface, error) {
	configClient, err := configclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}
	return configClient.ConfigV1(), nil
}

// FetchAPIServerTLSProfile fetches the TLS security profile from the cluster APIServer resource.
// Returns the Intermediate profile as default if no profile is specified.
func FetchAPIServerTLSProfile(ctx context.Context, configClient configv1client.ConfigV1Interface) (*configv1.TLSSecurityProfile, error) {
	apiserver, err := configClient.APIServers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer config: %w", err)
	}

	profile := apiserver.Spec.TLSSecurityProfile
	if profile == nil {
		// Use Intermediate profile as default when no profile is specified
		profile = &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
	}

	return profile, nil
}
