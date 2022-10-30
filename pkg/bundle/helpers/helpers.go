package helpers

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// GetBundleType returns the concrete type of a bundle.
func GetBundleType(bundle status.Bundle) string {
	array := strings.Split(fmt.Sprintf("%T", bundle), ".")
	return array[len(array)-1]
}
