package helper

import (
	"os"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	// start testEnv
	testEnv := &envtest.Environment{}
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}
	m.Run()
	// stop testEnv
	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}
}

func TestParseFlags(t *testing.T) {
	os.Args = []string{
		"agent",
		"--lease-duration",
		"137",
		"--renew-deadline",
		"107",
		"--leaf-hub-name",
		"hub1",
	}
	agentConfig, err := NewConfigManager()
	if err != nil {
		t.Fatalf("failed to parseFlags %v", err)
	}

	if agentConfig.ElectionConfig.LeaseDuration != 137 {
		t.Fatalf("expect --lease-duration(%d) == %d", agentConfig.ElectionConfig.LeaseDuration, 137)
	}

	if agentConfig.ElectionConfig.RenewDeadline != 107 {
		t.Fatalf("expect --lease-duration(%d) == %d", agentConfig.ElectionConfig.RenewDeadline, 107)
	}
}
