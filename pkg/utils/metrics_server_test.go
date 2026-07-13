/*
Copyright Contributors to the Open Cluster Management project.
*/

package utils

import (
	"crypto/tls"
	"testing"
)

func TestNewSecureMetricsServerOptions(t *testing.T) {
	tlsOpt := func(cfg *tls.Config) {
		cfg.MinVersion = tls.VersionTLS12
	}

	opts := NewSecureMetricsServerOptions(":8384", tlsOpt)
	if opts.BindAddress != ":8384" {
		t.Fatalf("expected BindAddress :8384, got %q", opts.BindAddress)
	}
	if !opts.SecureServing {
		t.Fatal("expected SecureServing true so TLSOpts apply")
	}
	if len(opts.TLSOpts) != 1 {
		t.Fatalf("expected 1 TLSOpt, got %d", len(opts.TLSOpts))
	}

	cfg := &tls.Config{}
	opts.TLSOpts[0](cfg)
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLSOpt to set MinVersion TLS1.2, got 0x%x", cfg.MinVersion)
	}
}

func TestNewSecureMetricsServerOptionsNoTLSOpts(t *testing.T) {
	opts := NewSecureMetricsServerOptions("0.0.0.0:8384")
	if !opts.SecureServing {
		t.Fatal("expected SecureServing true")
	}
	if opts.BindAddress != "0.0.0.0:8384" {
		t.Fatalf("unexpected BindAddress %q", opts.BindAddress)
	}
	if len(opts.TLSOpts) != 0 {
		t.Fatalf("expected no TLSOpts, got %d", len(opts.TLSOpts))
	}
}
