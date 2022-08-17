package helpers

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
)

// GetBundleType returns the concrete type of a bundle.
func GetBundleType(bundle bundle.Bundle) string {
	array := strings.Split(fmt.Sprintf("%T", bundle), ".")
	return array[len(array)-1]
}
