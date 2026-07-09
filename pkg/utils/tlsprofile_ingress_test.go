/*
Copyright Contributors to the Open Cluster Management project.
*/

package utils

import (
	"context"
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func ingressTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := configv1.AddToScheme(s); err != nil {
		t.Fatalf("add configv1 scheme: %v", err)
	}
	if err := operatorv1.Install(s); err != nil {
		t.Fatalf("add operatorv1 scheme: %v", err)
	}
	return s
}

func TestFetchIngressControllerTLSProfileSpecFromStatus(t *testing.T) {
	s := ingressTestScheme(t)
	ic := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultIngressControllerName,
			Namespace: defaultIngressControllerNamespace,
		},
		Status: operatorv1.IngressControllerStatus{
			TLSProfile: &configv1.TLSProfileSpec{
				Ciphers:       []string{testCipherECDHERSAAES128},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}
	c := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(ic).Build()

	spec, err := FetchIngressControllerTLSProfileSpec(context.Background(), c)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	if spec.MinTLSVersion != configv1.VersionTLS12 {
		t.Fatalf("expected MinTLSVersion %s, got %s", configv1.VersionTLS12, spec.MinTLSVersion)
	}
	if len(spec.Ciphers) != 1 || spec.Ciphers[0] != testCipherECDHERSAAES128 {
		t.Fatalf("unexpected ciphers: %#v", spec.Ciphers)
	}
}

func TestFetchIngressControllerTLSProfileSpecFromSpec(t *testing.T) {
	s := ingressTestScheme(t)
	ic := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultIngressControllerName,
			Namespace: defaultIngressControllerNamespace,
		},
		Spec: operatorv1.IngressControllerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}
	c := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(ic).Build()

	spec, err := FetchIngressControllerTLSProfileSpec(context.Background(), c)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	expected := configv1.TLSProfiles[configv1.TLSProfileModernType]
	if spec.MinTLSVersion != expected.MinTLSVersion {
		t.Fatalf("expected MinTLSVersion %s, got %s", expected.MinTLSVersion, spec.MinTLSVersion)
	}
}

func TestFetchIngressControllerTLSProfileSpecFallsBackToAPIServer(t *testing.T) {
	s := ingressTestScheme(t)
	ic := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultIngressControllerName,
			Namespace: defaultIngressControllerNamespace,
		},
	}
	apiserver := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
		},
	}
	c := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(ic, apiserver).Build()

	spec, err := FetchIngressControllerTLSProfileSpec(context.Background(), c)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	expected := configv1.TLSProfiles[configv1.TLSProfileOldType]
	if spec.MinTLSVersion != expected.MinTLSVersion {
		t.Fatalf("expected MinTLSVersion %s, got %s", expected.MinTLSVersion, spec.MinTLSVersion)
	}
}

func TestFetchIngressControllerTLSProfileSpecMissingDefaultsToIntermediate(t *testing.T) {
	s := ingressTestScheme(t)
	c := fakeclient.NewClientBuilder().WithScheme(s).Build()

	spec, err := FetchIngressControllerTLSProfileSpec(context.Background(), c)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	expected := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	if spec.MinTLSVersion != expected.MinTLSVersion {
		t.Fatalf("expected Intermediate MinTLSVersion %s, got %s",
			expected.MinTLSVersion, spec.MinTLSVersion)
	}
}

func TestGetTLSConfigFromIngressController(t *testing.T) {
	s := ingressTestScheme(t)
	ic := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultIngressControllerName,
			Namespace: defaultIngressControllerNamespace,
		},
		Status: operatorv1.IngressControllerStatus{
			TLSProfile: &configv1.TLSProfileSpec{
				Ciphers:       []string{testCipherECDHERSAAES128},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}
	c := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(ic).Build()

	cfg, err := GetTLSConfigFromIngressController(context.Background(), c)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2, got 0x%x", cfg.MinVersion)
	}
	if len(cfg.CipherSuites) != 1 {
		t.Fatalf("expected 1 cipher suite, got %d", len(cfg.CipherSuites))
	}
}

func TestBuildTLSConfigFromIngressProfileSpecNilDefaults(t *testing.T) {
	cfg, err := BuildTLSConfigFromIngressProfileSpec(nil)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	expected, err := resolveSpec(nil)
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	minVer, err := parseTLSVersion(string(expected.MinTLSVersion))
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	if cfg.MinVersion != minVer {
		t.Fatalf("expected MinVersion 0x%x, got 0x%x", minVer, cfg.MinVersion)
	}
}
