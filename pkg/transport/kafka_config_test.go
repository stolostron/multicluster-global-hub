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
