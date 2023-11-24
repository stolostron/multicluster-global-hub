package syncers

import (
	"encoding/json"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/hubcluster"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// managedClusterLabelsBundleSyncer syncs managed clusters metadata from received bundles.
type resendSyncer struct {
	log logr.Logger
}

func NewResendSyncer() *resendSyncer {
	return &resendSyncer{
		log: ctrl.Log.WithName("Resend status syncer"),
	}
}

func (syncer *resendSyncer) Sync(payload []byte) error {
	syncer.log.Info("Resend bundle info")
	bundleKeys := []string{}
	if err := json.Unmarshal(payload, &bundleKeys); err != nil {
		syncer.log.Error(err, "Failed to unmarshal bundle keys")
		return err
	}

	for _, bundleKey := range bundleKeys {
		syncer.log.Info("Resend bundle", "key", bundleKey)
		if bundleKey == constants.HubClusterInfoMsgKey {
			hubcluster.IncreaseBundleVersion()
			continue
		}
	}
	return nil
}
