package conflator

import (
	"testing"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stretchr/testify/assert"
)

func TestBundleInfoSetVersion(t *testing.T) {
	completeBundleInfo := &completeStateBundleInfo{
		bundle: &statusBundle{
			Incarnation: 1,
			Generation:  1,
		},
		metadata: nil,
	}
	completeBundleInfo.resetBundleVersion()
	assert.Equal(t, uint64(0), completeBundleInfo.bundle.GetVersion().Incarnation)
	assert.Equal(t, uint64(0), completeBundleInfo.bundle.GetVersion().Generation)

	deltaBundleInfo := &deltaStateBundleInfo{
		bundle: &statusBundle{
			Incarnation: 2,
			Generation:  2,
		},
		metadata: nil,
		lastDispatchedDeltaBundleData: recoverableDeltaStateBundleData{
			bundle:                         nil,
			lowestPendingTransportMetadata: nil,
		},
		lastReceivedTransportMetadata: nil,
		deltaLineHeadBundleVersion:    nil,
	}
	deltaBundleInfo.resetBundleVersion()
	assert.Equal(t, uint64(0), deltaBundleInfo.bundle.GetVersion().Incarnation)
	assert.Equal(t, uint64(0), deltaBundleInfo.bundle.GetVersion().Generation)
}

type statusBundle struct {
	Incarnation uint64
	Generation  uint64
}

func (b *statusBundle) GetLeafHubName() string {
	return "testHubName"
}

func (b *statusBundle) GetObjects() []interface{} {
	return []interface{}{}
}

func (b *statusBundle) GetVersion() *status.BundleVersion {
	return &status.BundleVersion{
		Incarnation: b.Incarnation,
		Generation:  b.Generation,
	}
}

func (b *statusBundle) SetVersion(version *status.BundleVersion) {
	b.Incarnation = version.Incarnation
	b.Generation = version.Generation
}

// for delta bundle
func (b *statusBundle) GetDependencyVersion() *status.BundleVersion {
	return &status.BundleVersion{
		Incarnation: 1,
		Generation:  1,
	}
}

func (b *statusBundle) InheritEvents(olderBundle status.Bundle) error {
	return nil
}
