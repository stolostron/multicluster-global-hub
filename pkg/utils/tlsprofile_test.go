package utils

import (
	"context"
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

const (
	testCipherECDHERSAAES128 = "ECDHE-RSA-AES128-GCM-SHA256"
	testCipherUnsupported    = "UNSUPPORTED-CIPHER"
	testProfileTypeOld       = "Old profile type"
	testErrExpectedError     = "expected error but got none"
	testErrUnexpected        = "unexpected error: %v"
)

// TestGetTLSConfigFromAPIServer, GetTLSConfigFromClient, GetOpenShiftConfigClient,
// and FetchAPIServerTLSProfile require real Kubernetes clients and are tested in integration tests.

func TestResolveSpec(t *testing.T) {
	tests := []struct {
		name        string
		profile     *configv1.TLSSecurityProfile
		expectError bool
		expectType  configv1.TLSProfileType
	}{
		{
			name:        "nil profile defaults to Intermediate",
			profile:     nil,
			expectError: false,
			expectType:  configv1.TLSProfileIntermediateType,
		},
		{
			name: testProfileTypeOld,
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectError: false,
			expectType:  configv1.TLSProfileOldType,
		},
		{
			name: "Intermediate profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectError: false,
			expectType:  configv1.TLSProfileIntermediateType,
		},
		{
			name: "Modern profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectError: false,
			expectType:  configv1.TLSProfileModernType,
		},
		{
			name: "Custom profile with valid spec",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{testCipherECDHERSAAES128},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError: false,
		},
		{
			name: "Custom profile with nil Custom field",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			expectError: true,
		},
		{
			name: "Unknown profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: "UnknownType",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := resolveSpec(tt.profile)
			if tt.expectError {
				if err == nil {
					t.Error(testErrExpectedError)
				}
				return
			}
			if err != nil {
				t.Errorf(testErrUnexpected, err)
				return
			}
			if spec == nil {
				t.Errorf("expected spec but got nil")
				return
			}
			if tt.expectType != "" {
				expected := configv1.TLSProfiles[tt.expectType]
				if spec.MinTLSVersion != expected.MinTLSVersion {
					t.Fatalf("expected MinTLSVersion %q, got %q", expected.MinTLSVersion, spec.MinTLSVersion)
				}
			}
		})
	}
}

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expected    uint16
		expectError bool
	}{
		{
			name:        "TLS 1.0",
			version:     "VersionTLS10",
			expected:    tls.VersionTLS10,
			expectError: false,
		},
		{
			name:        "TLS 1.1",
			version:     "VersionTLS11",
			expected:    tls.VersionTLS11,
			expectError: false,
		},
		{
			name:        "TLS 1.2",
			version:     "VersionTLS12",
			expected:    tls.VersionTLS12,
			expectError: false,
		},
		{
			name:        "TLS 1.3",
			version:     "VersionTLS13",
			expected:    tls.VersionTLS13,
			expectError: false,
		},
		{
			name:        "Unknown version",
			version:     "VersionTLS99",
			expectError: true,
		},
		{
			name:        "Invalid format",
			version:     "TLS1.2",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTLSVersion(tt.version)
			if tt.expectError {
				if err == nil {
					t.Error(testErrExpectedError)
				}
				return
			}
			if err != nil {
				t.Errorf(testErrUnexpected, err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %d but got %d", tt.expected, result)
			}
		})
	}
}

func TestMapCipherSuites(t *testing.T) {
	tests := []struct {
		name     string
		ciphers  []string
		expected int // expected number of valid cipher suites
	}{
		{
			name:     "empty list",
			ciphers:  []string{},
			expected: 0,
		},
		{
			name:     "all valid ciphers",
			ciphers:  []string{testCipherECDHERSAAES128, "ECDHE-ECDSA-AES128-GCM-SHA256"},
			expected: 2,
		},
		{
			name:     "mixed valid and invalid",
			ciphers:  []string{testCipherECDHERSAAES128, testCipherUnsupported},
			expected: 1,
		},
		{
			name:     "all unsupported",
			ciphers:  []string{"DHE-RSA-AES256-SHA", testCipherUnsupported},
			expected: 0,
		},
		{
			name:     "TLS 1.3 ciphers (should be supported)",
			ciphers:  []string{"ECDHE-RSA-CHACHA20-POLY1305", "ECDHE-ECDSA-CHACHA20-POLY1305"},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapCipherSuites(tt.ciphers)
			if len(result) != tt.expected {
				t.Errorf("expected %d cipher suites but got %d", tt.expected, len(result))
			}
		})
	}
}

type tlsConfigTestCase struct {
	name              string
	profile           *configv1.TLSSecurityProfile
	expectError       bool
	checkMinVer       bool
	expectedMin       uint16
	checkCipherNil    bool
	checkCipherNonNil bool
}

func getBuildTLSConfigTestCases() []tlsConfigTestCase {
	return []tlsConfigTestCase{
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS12,
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS13,
		},
		{
			name: "Custom profile with TLS 1.2",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{testCipherECDHERSAAES128},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS12,
		},
		{
			name: "Custom profile with all unsupported ciphers",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"UNSUPPORTED-CIPHER-1", "UNSUPPORTED-CIPHER-2"},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError: true,
		},
		{
			name:        "nil profile defaults to Intermediate",
			profile:     nil,
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS12,
		},
		{
			name: "Custom profile with empty ciphers uses Go defaults",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError:    false,
			checkMinVer:    true,
			expectedMin:    tls.VersionTLS12,
			checkCipherNil: true,
		},
		{
			name: "Invalid TLS version",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{testCipherECDHERSAAES128},
						MinTLSVersion: "BadVersion",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Old profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS10,
		},
		{
			name: "TLS 1.3 with cipher list (ciphers not set)",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"ANY-CIPHER"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS13,
		},
	}
}

func TestBuildTLSConfig(t *testing.T) {
	tests := getBuildTLSConfigTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildTLSConfig(tt.profile)
			if tt.expectError {
				if err == nil {
					t.Error(testErrExpectedError)
				}
				return
			}
			if err != nil {
				t.Errorf(testErrUnexpected, err)
				return
			}
			if cfg == nil {
				t.Errorf("expected tls.Config but got nil")
				return
			}
			if tt.checkMinVer && cfg.MinVersion != tt.expectedMin {
				t.Errorf("expected MinVersion %d but got %d", tt.expectedMin, cfg.MinVersion)
			}
			if tt.checkCipherNil && cfg.CipherSuites != nil {
				t.Errorf("expected nil CipherSuites for Go defaults, got %v", cfg.CipherSuites)
			}
			if tt.checkCipherNonNil && len(cfg.CipherSuites) == 0 {
				t.Error("expected non-empty CipherSuites")
			}
		})
	}
}

func getBuildTLSConfigFuncTestCases() []tlsConfigTestCase {
	return []tlsConfigTestCase{
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS12,
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS13,
		},
		{
			name: "Custom profile with unsupported ciphers for TLS 1.2",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"UNSUPPORTED-1", "UNSUPPORTED-2"},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError: true,
		},
		{
			name: "TLS 1.3 profile with cipher list (should succeed, ciphers ignored)",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{testCipherUnsupported},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS13,
		},
		{
			name:        "nil profile",
			profile:     nil,
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS12,
		},
		{
			name: "Invalid TLS version in profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{testCipherECDHERSAAES128},
						MinTLSVersion: "InvalidVersion",
					},
				},
			},
			expectError: true,
		},
		{
			name: "TLS 1.2 with empty cipher list",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			expectError:    false,
			checkMinVer:    true,
			expectedMin:    tls.VersionTLS12,
			checkCipherNil: true,
		},
		{
			name: "Old profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectError: false,
			checkMinVer: true,
			expectedMin: tls.VersionTLS10,
		},
	}
}

func TestBuildTLSConfigFunc(t *testing.T) {
	tests := getBuildTLSConfigFuncTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFunc, err := BuildTLSConfigFunc(tt.profile)
			if tt.expectError {
				if err == nil {
					t.Error(testErrExpectedError)
				}
				return
			}
			if err != nil {
				t.Errorf(testErrUnexpected, err)
				return
			}
			if configFunc == nil {
				t.Errorf("expected config function but got nil")
				return
			}

			// Apply the function to a new tls.Config
			cfg := &tls.Config{}
			configFunc(cfg)

			if tt.checkMinVer && cfg.MinVersion != tt.expectedMin {
				t.Errorf("expected MinVersion %d but got %d", tt.expectedMin, cfg.MinVersion)
			}
			if tt.checkCipherNil && cfg.CipherSuites != nil {
				t.Errorf("expected nil CipherSuites for Go defaults, got %v", cfg.CipherSuites)
			}
		})
	}
}

func TestTLS13OnlyConfigFunc(t *testing.T) {
	cfg := &tls.Config{}
	TLS13OnlyConfigFunc()(cfg)
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS 1.3, got %d", cfg.MinVersion)
	}
}

func TestBuildTLSConfigFuncFromProfile(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
	tlsConfigFunc, profileType, err := buildTLSConfigFuncFromProfile(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profileType != configv1.TLSProfileIntermediateType {
		t.Fatalf("expected profile type %q, got %q", configv1.TLSProfileIntermediateType, profileType)
	}
	cfg := &tls.Config{}
	tlsConfigFunc(cfg)
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
}

func TestBuildTLSConfigFuncFromProfileUnsupportedCiphers(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       []string{"UNSUPPORTED-CIPHER"},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}
	_, _, err := buildTLSConfigFuncFromProfile(profile)
	if err == nil {
		t.Fatal("expected error for unsupported cipher profile")
	}
}

func TestBuildMetricsTLSConfigFuncFallback(t *testing.T) {
	tlsConfigFunc, profileType, err := BuildMetricsTLSConfigFunc(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profileType != "" {
		t.Fatalf("expected empty profile type on fallback, got %q", profileType)
	}
	cfg := &tls.Config{}
	tlsConfigFunc(cfg)
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS 1.3, got %d", cfg.MinVersion)
	}
}
