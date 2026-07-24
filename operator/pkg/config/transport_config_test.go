package config

import "testing"

func TestGetMigrationTopic(t *testing.T) {
	original := migrationTopic
	t.Cleanup(func() { migrationTopic = original })

	migrationTopic = "gh-migration"
	if got := GetMigrationTopic(); got != "gh-migration" {
		t.Fatalf("GetMigrationTopic() = %q, want gh-migration", got)
	}
}

func TestGetKafkaUserName(t *testing.T) {
	if got := GetKafkaUserName("hub1"); got != "hub1-kafka-user" {
		t.Fatalf("GetKafkaUserName() = %q, want hub1-kafka-user", got)
	}
}

func TestSetMigrationTopic(t *testing.T) {
	original := migrationTopic
	t.Cleanup(func() { migrationTopic = original })

	SetMigrationTopic("custom-migration")
	if got := GetMigrationTopic(); got != "custom-migration" {
		t.Fatalf("SetMigrationTopic() = %q, want custom-migration", got)
	}
}
