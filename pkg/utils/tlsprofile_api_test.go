package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func newTestAPIServerHTTPServer(t *testing.T, profileType configv1.TLSProfileType) *httptest.Server {
	t.Helper()
	apiserver := configv1.APIServer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configv1.SchemeGroupVersion.String(),
			Kind:       "APIServer",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: profileType},
		},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet ||
			r.URL.Path != "/apis/config.openshift.io/v1/apiservers/cluster" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(apiserver); err != nil {
			t.Errorf("failed to encode APIServer: %v", err)
		}
	}))
}

func testRestConfig(host string) *rest.Config {
	return &rest.Config{
		Host: host,
		ContentConfig: rest.ContentConfig{
			GroupVersion: &configv1.SchemeGroupVersion,
		},
	}
}

func TestGetTLSConfigFromAPIServer(t *testing.T) {
	srv := newTestAPIServerHTTPServer(t, configv1.TLSProfileModernType)
	defer srv.Close()

	cfg, err := GetTLSConfigFromAPIServer(context.Background(), testRestConfig(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS 1.3, got %d", cfg.MinVersion)
	}
}

func TestGetOpenShiftConfigClientSuccess(t *testing.T) {
	srv := newTestAPIServerHTTPServer(t, configv1.TLSProfileIntermediateType)
	defer srv.Close()

	client, err := GetOpenShiftConfigClient(testRestConfig(srv.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	profile, err := FetchAPIServerTLSProfile(context.Background(), client.APIServers())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Type != configv1.TLSProfileIntermediateType {
		t.Fatalf("expected Intermediate profile, got %q", profile.Type)
	}
}

func TestBuildMetricsTLSConfigFuncWithAPIServer(t *testing.T) {
	srv := newTestAPIServerHTTPServer(t, configv1.TLSProfileIntermediateType)
	defer srv.Close()

	tlsConfigFunc, profileType, err := BuildMetricsTLSConfigFunc(
		context.Background(),
		testRestConfig(srv.URL),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profileType != configv1.TLSProfileIntermediateType {
		t.Fatalf("expected Intermediate profile, got %q", profileType)
	}
	cfg := &tls.Config{}
	tlsConfigFunc(cfg)
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
}

func TestGetTLSConfigFromAPIServerInvalidHost(t *testing.T) {
	_, err := GetTLSConfigFromAPIServer(context.Background(), &rest.Config{Host: "://invalid-host"})
	if err == nil {
		t.Fatal("expected error for invalid rest config host")
	}
}
