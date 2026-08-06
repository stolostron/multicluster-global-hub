// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import "testing"

func TestKafkaConfigGetMigrationTopic(t *testing.T) {
	cfg := &KafkaConfig{
		SpecTopic:      "gh-spec",
		MigrationTopic: "gh-migration",
	}
	if got := cfg.GetMigrationTopic(); got != "gh-migration" {
		t.Fatalf("GetMigrationTopic() = %q, want gh-migration", got)
	}

	fallback := &KafkaConfig{SpecTopic: "gh-spec"}
	if got := fallback.GetMigrationTopic(); got != "gh-spec" {
		t.Fatalf("GetMigrationTopic() fallback = %q, want gh-spec", got)
	}

	var nilCfg *KafkaConfig
	if got := nilCfg.GetMigrationTopic(); got != "" {
		t.Fatalf("GetMigrationTopic() nil = %q, want empty string", got)
	}
}

func TestKafkaConfigDeepCopyAndYamlMarshal(t *testing.T) {
	cfg := &KafkaConfig{
		BootstrapServer:  "kafka:9093",
		SpecTopic:        "gh-spec",
		MigrationTopic:   "gh-migration",
		StatusTopic:      "gh-status.hub1",
		CACert:           "ca",
		ClientCert:       "cert",
		ClientKey:        "key",
		CASecretName:     "ca-secret",
		ClientSecretName: "client-secret",
	}
	copy := cfg.DeepCopy()
	if copy == cfg || copy.SpecTopic != cfg.SpecTopic {
		t.Fatal("DeepCopy() did not produce an independent copy")
	}

	rawYAML, err := cfg.YamlMarshal(true)
	if err != nil {
		t.Fatalf("YamlMarshal(true) error = %v", err)
	}
	if string(rawYAML) == "" {
		t.Fatal("YamlMarshal(true) returned empty output")
	}

	secretYAML, err := cfg.YamlMarshal(false)
	if err != nil {
		t.Fatalf("YamlMarshal(false) error = %v", err)
	}
	if string(secretYAML) == "" {
		t.Fatal("YamlMarshal(false) returned empty output")
	}
}

func TestKafkaConfigCertAccessors(t *testing.T) {
	cfg := &KafkaConfig{}
	cfg.SetCACert("ca")
	cfg.SetClientCert("cert")
	cfg.SetClientKey("key")

	if got := cfg.GetCACert(); got != "ca" {
		t.Fatalf("GetCACert() = %q, want ca", got)
	}
	if got := cfg.GetClientCert(); got != "cert" {
		t.Fatalf("GetClientCert() = %q, want cert", got)
	}
	if got := cfg.GetClientKey(); got != "key" {
		t.Fatalf("GetClientKey() = %q, want key", got)
	}
	if got := cfg.GetCASecretName(); got != "" {
		t.Fatalf("GetCASecretName() = %q, want empty", got)
	}
	if got := cfg.GetClientSecretName(); got != "" {
		t.Fatalf("GetClientSecretName() = %q, want empty", got)
	}
}
